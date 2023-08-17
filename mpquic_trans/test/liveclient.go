package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	log "github.com/sirupsen/logrus"
	quic "mp_quic"
	proxy "mp_quic/liveproxy"
	"mp_quic/liveproxy/configure"
	"mp_quic/liveproxy/protocol/rtmp"
	"mp_quic/liveproxy/protocol/rtmp/core"
	"time"
)

const (
	serverIP  = "localhost"
	quicPort  = "4242"
	multipath = true
	//serverAddr = "localhost:4242"
	maxPLen = 1024
)

func liveClient() {
	session, err := quic.DialAddr(serverIP+":"+quicPort, &tls.Config{InsecureSkipVerify: true}, &quic.Config{CreatePaths: multipath})
	if err != nil {
		log.Error("liveclient panic_session: ", err)
	}

	//session.InteractWithModel()

	var stream quic.Stream
	for {
		stream, err = session.OpenStreamSync()
		if err != nil {
			log.Error("liveclient panic_stream: ", err)
		} else {
			break
		}
	}

	defer func() {
		err = stream.Close()
		if err != nil {
			log.Error("liveclient panic_stream: ", err)
		}
	}()

	rtmp.SetRole(rtmp.CLIENT_IND)
	for {
		var frame core.ChunkStream
		frame = rtmp.GetRTMPFrame(frame)

		metaBuf := bytes.NewBuffer([]byte{})
		err := binary.Write(metaBuf, binary.BigEndian, frame.Timestamp)
		if err != nil {
			log.Error("liveclient panic_metadata: ", err)
		}
		err = binary.Write(metaBuf, binary.BigEndian, frame.Length)
		if err != nil {
			log.Error("liveclient panic_metadata: ", err)
		}
		err = binary.Write(metaBuf, binary.BigEndian, frame.StreamID)
		if err != nil {
			log.Error("liveclient panic_metadata: ", err)
		}
		err = binary.Write(metaBuf, binary.BigEndian, frame.TypeID)
		if err != nil {
			log.Error("liveclient panic_metadata: ", err)
		}
		_, err = stream.Write(metaBuf.Bytes())
		if err != nil {
			log.Error("liveclient panic_metadata: ", err)
		} else {
			pNum := int(frame.Length / maxPLen)
			for i := 0; i < pNum; i++ {
				_, err = stream.Write(frame.Data[i*maxPLen : (i+1)*maxPLen])
				if err != nil {
					log.Error("liveclient panic_data: ", err)
				}
			}

			_, err = stream.Write(frame.Data[pNum*maxPLen : int(frame.Length)])
			if err != nil {
				log.Error("liveclient panic_data: ", err)
			}

			log.Infof("quic packet size=%d", frame.Length)
		}
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Error("liveclient panic: ", r)
			time.Sleep(1 * time.Second)
		}
	}()

	go liveClient()

	apps := configure.Applications{}
	configure.Config.UnmarshalKey("server", &apps)
	for _, app := range apps {
		rtmpStream := rtmp.NewRtmpStream()
		if app.Api {
			proxy.StartAPI(rtmpStream, "", "")
		}
		proxy.StartRtmp(rtmpStream, "")
	}
}
