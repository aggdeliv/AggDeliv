# MPQUIC

### QUIC Client
We execute the program with `go run liveclient.go` (in the *mp_quic/test* dir), which retrieves RTMP chunks from the RTMP stream and transforms them into QUIC packets, and then calls `client_wrapper.go` to send the packets to **QUIC Client**. Meanwhile, `client_wrapper.go` will call the python program *server* in the *packet_scheduling* dir (should be run first with `python server.py`) through *grpc* to acquire choice (probabilities) of tranfer path for each QUIC packet.

***Note***: The "ServerIP" variable should be changed to real IP of **QUIC Server**.

### QUIC Server
We execute the program with `go run liveserver.go` (in the *mp_quic/test* dir), which calls `server_wrapper.go` to receive QUIC packets from **QUIC Client** and then transforms the packets to RTMP chunks and adds them into the RTMP stream.

***Note***: We should first start **QUIC Server** and then run **QUIC Client**.

### Demo Video for Test
To test data transmission between **QUIC Client** and **QUIC Server**, we can push a RTMP stream to **QUIC Client** by executing a *FFmpeg* command `ffmpeg -re -i {dir/}demo.flv -c copy -f flv rtmp://localhost:1935/live/{channelkey}` (a demo video *demo.flv* is provided in the *mp_quic* dir). Before executing the command, we need to acquire a *channel key* from **QUIC Client** by conducting `HTTP Get http://localhost:8090/control/reset?room=movie` and retrieving the last segment of its response.
