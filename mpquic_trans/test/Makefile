update:
	$(MAKE) -C .. update
qpserver:
	go run qpserver.go -v -l :3443 -cert ./cert/cert.pem -key ./cert/domain.key -auth username:password
qpclient:
	go run qpclient.go -v -k -proxy http://192.168.1.137:3443 -l 127.0.0.1:18080 -auth username:password
