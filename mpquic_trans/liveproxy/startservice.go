package liveproxy

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"mp_quic/liveproxy/configure"
	"mp_quic/liveproxy/protocol/api"
	"mp_quic/liveproxy/protocol/hls"
	"mp_quic/liveproxy/protocol/httpflv"
	"mp_quic/liveproxy/protocol/rtmp"
	"net"
	"path"
	"runtime"
)

//var VERSION = "master"

//const RTMP_STREAM = "rtmp://localhost:1935/live/movie"
//type ChunkMeta struct {
//	Timestamp uint32
//	Length    uint32
//	TypeID    uint32
//	StreamID  uint32
//}

func StartHls() *hls.Server {
	hlsAddr := configure.Config.GetString("hls_addr")
	hlsListen, err := net.Listen("tcp", hlsAddr)
	if err != nil {
		log.Fatal(err)
	}

	hlsServer := hls.NewServer()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HLS server panic: ", r)
			}
		}()
		log.Info("HLS listen On ", hlsAddr)
		hlsServer.Serve(hlsListen)
	}()
	return hlsServer
}

//func StartRtmp(stream *rtmp.RtmpStream, hlsServer *hls.Server) {
func StartRtmp(stream *rtmp.RtmpStream, rtmpAddr string) {
	if rtmpAddr == "" {
		rtmpAddr = configure.Config.GetString("rtmp_addr")
	}

	rtmpListen, err := net.Listen("tcp", rtmpAddr)
	if err != nil {
		log.Fatal(err)
	}

	var rtmpServer *rtmp.Server
	rtmpServer = rtmp.NewRtmpServer(stream, nil)

	//if hlsServer == nil {
	//	rtmpServer = rtmp.NewRtmpServer(stream, nil)
	//	log.Info("HLS server disable....")
	//} else {
	//	rtmpServer = rtmp.NewRtmpServer(stream, hlsServer)
	//	log.Info("HLS server enable....")
	//}

	defer func() {
		if r := recover(); r != nil {
			log.Error("RTMP server panic: ", r)
		}
	}()
	log.Info("RTMP Listen On ", rtmpAddr)
	rtmpServer.Serve(rtmpListen)
}

func StartHTTPFlv(stream *rtmp.RtmpStream) {
	httpflvAddr := configure.Config.GetString("httpflv_addr")

	flvListen, err := net.Listen("tcp", httpflvAddr)
	if err != nil {
		log.Fatal(err)
	}

	hdlServer := httpflv.NewServer(stream)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HTTP-FLV server panic: ", r)
			}
		}()
		log.Info("HTTP-FLV listen On ", httpflvAddr)
		hdlServer.Serve(flvListen)
	}()
}

func StartAPI(stream *rtmp.RtmpStream, apiAddr, rtmpAddr string) {
	if apiAddr == "" && rtmpAddr == "" {
		apiAddr = configure.Config.GetString("api_addr")
		rtmpAddr = configure.Config.GetString("rtmp_addr")
	}

	if apiAddr != "" {
		opListen, err := net.Listen("tcp", apiAddr)
		if err != nil {
			log.Fatal(err)
		}
		opServer := api.NewServer(stream, rtmpAddr)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("HTTP-API server panic: ", r)
				}
			}()
			log.Info("HTTP-API listen On ", apiAddr)
			opServer.Serve(opListen)
		}()
	}
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})
}

//func main() {
//	defer func() {
//		if r := recover(); r != nil {
//			log.Error("liveproxy panic: ", r)
//			time.Sleep(1 * time.Second)
//		}
//	}()
//
//	log.Infof(`
//     _     _            ____
//    | |   (_)_   _____ / ___| ___
//    | |   | \ \ / / _ \ |  _ / _ \
//    | |___| |\ V /  __/ |_| | (_) |
//    |_____|_| \_/ \___|\____|\___/
//        version: %s
//	`, VERSION)
//
//	apps := configure.Applications{}
//	configure.Config.UnmarshalKey("server", &apps)
//	for _, app := range apps {
//		stream := rtmp.NewRtmpStream()
//		var hlsServer *hls.Server
//		if app.Hls {
//			hlsServer = startHls()
//		}
//		if app.Flv {
//			startHTTPFlv(stream)
//		}
//		if app.Api {
//			startAPI(stream)
//		}
//
//		startRtmp(stream, hlsServer)
//	}

//apps := configure.Applications{}
//configure.Config.UnmarshalKey("server", &apps)
//for _, _ = range apps {
//	stream := rtmp.NewRtmpStream()
//	startRtmp(stream, nil)
//}

//buf := bytes.NewBuffer(nil)
//err := ffmpeg.Input(RTMP_STREAM).
//	Filter("select", ffmpeg.Args{fmt.Sprintf("gte(n,%d)", 1)}).
//	Output("pipe:", ffmpeg.KwArgs{"vframes": 1, "format": "image2", "vcodec": "mjpeg"}).
//	WithOutput(buf, os.Stdout).
//	Run()
//if err != nil {
//	panic(err)
//}
//}
