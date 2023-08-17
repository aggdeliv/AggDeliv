package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	quic "mp_quic"
	"mp_quic/liveproxy/av"
	"mp_quic/liveproxy/protocol/rtmp/core"
	"time"

	log "github.com/sirupsen/logrus"
	proxy "mp_quic/liveproxy"
	"mp_quic/liveproxy/configure"
	"mp_quic/liveproxy/protocol/rtmp"
)

const (
	maxSID         = 1000
	serverRtmpAddr = ":19350"
	dialUrl        = "rtmp://localhost:19350/live/movie"
	localAddr      = "localhost:4242"
	metadataLen    = 16
	maxPacketLen   = 1024
)

func liveServer() {
	time.Sleep(2 * time.Second)
	rtmp.SetRole(rtmp.SERVER_IND)
	var rtmpClient *rtmp.Client
	rtmpStream := rtmp.NewRtmpStream()
	rtmpClient = rtmp.NewRtmpClient(rtmpStream, nil)
	err := rtmpClient.Dial(dialUrl, av.PUBLISH)
	if err != nil {
		log.Error("liveserver panic_rtmp: ", err)
	}

	listener, err := quic.ListenAddr(localAddr, generateTLSConfig2(), nil)
	if err != nil {
		log.Error("liveserver panic_session: ", err)
	}
	sess, err := listener.Accept()
	if err != nil {
		log.Error("liveserver panic_session: ", err)
	}
	var stream quic.Stream
	for {
		stream, err = sess.AcceptStream()
		if err != nil {
			log.Error("liveserver panic_stream: ", err)
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	defer func() {
		err = stream.Close()
		if err != nil {
			log.Error("server wrapper panic_stream: ", err)
		}
	}()

	for {
		metaBuf := make([]byte, metadataLen)
		_, err := stream.Read(metaBuf)
		if err != nil {
			//quicStream.Close()
			log.Error("liveserver panic_metadata: ", err)
		} else {
			var metaItem [metadataLen / 4]uint32
			for i := 0; i < metadataLen/4; i++ {
				err = binary.Read(bytes.NewBuffer(metaBuf[4*i:4*i+4]), binary.BigEndian, &metaItem[i])
				if err != nil {
					log.Error("liveserver panic_metadata: ", err)
				}
			}
			dataLen := metaItem[1]
			//sendResponse(metaItem[0], uint32(0), recvTime)

			var data []byte
			if dataLen < maxPacketLen {
				data = make([]byte, dataLen)
				_, err := stream.Read(data)
				if err != nil {
					log.Error("liveserver panic_data: ", err)
				}
			} else {
				totalRead := 0
				data = make([]byte, maxPacketLen)
				read, err := stream.Read(data)
				if err != nil {
					log.Error("liveserver panic_data: ", err)
				}

				tempLen := maxPacketLen
				for {
					totalRead += read
					if totalRead == int(dataLen) {
						break
					}
					if int(dataLen)-totalRead < maxPacketLen {
						tempLen = int(dataLen) - totalRead
					}
					temp := make([]byte, tempLen)
					read, err = stream.Read(temp)
					if err != nil {
						log.Error("liveserver panic_data: ", err)
					} else {
						data = append(data[:totalRead], temp[:read]...)
					}
				}
			}

			if len(data) > 0 {
				var frame core.ChunkStream
				frame.Timestamp = metaItem[0]
				frame.Length = metaItem[1]
				frame.StreamID = metaItem[2]
				frame.TypeID = metaItem[3]
				frame.Data = data
				log.Infof("real quic packet size=%d", len(frame.Data))
				if int(frame.Length) != len(frame.Data) {
					log.Infof("quic packet size=%d", frame.Length)
				}
				rtmp.SetRTMPFrame(frame)
			}
		}
	}
}

func generateTLSConfig2() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		log.Error("liveserver panic_tls: ", err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		log.Error("liveserver panic_tls: ", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		log.Error("liveserver panic_tls: ", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Error("liveserver panic_main: ", r)
			time.Sleep(1 * time.Second)
		}
	}()

	go liveServer()

	apps := configure.Applications{}
	configure.Config.UnmarshalKey("server", &apps)
	for _, app := range apps {
		rtmpStream := rtmp.NewRtmpStream()
		//if app.Api {
		//	proxy.StartAPI(rtmpStream, serverApiAddr, serverRtmpAddr)
		//}
		if app.Flv {
			proxy.StartHTTPFlv(rtmpStream)
		}
		proxy.StartRtmp(rtmpStream, serverRtmpAddr)
	}
}
