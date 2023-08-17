package qproxy

import (
	"net/http"

	"github.com/elazarl/goproxy"
)

func SetAuthForBasicRequest(username, password string) goproxy.ReqHandler {
	return goproxy.FuncReqHandler(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		SetBasicAuth(username, password, req)
		return req, nil
	})
}

func SetAuthForBasicConnectRequest(username, password string) func(req *http.Request) {
	return func(req *http.Request) {
		SetBasicAuth(username, password, req)
	}
}
