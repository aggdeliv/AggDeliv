# AggDeliv

### AggDeliv Client
We execute the program with `go run liveclient.go` (in the *mp_quic/test* dir), which retrieves RTMP chunks from the RTMP stream and transforms them into QUIC packets, and then calls `client_wrapper.go` to send the packets to **AggDeliv Client**. Meanwhile, `client_wrapper.go` will call the python program *server* in the *packet_scheduling* dir (should be run first with `python server.py`) through *grpc* to acquire choice (probabilities) of tranfer path for each QUIC packet.

***Note***: The "ServerIP" variable should be changed to real IP of **AggDeliv Server**.

### AggDeliv Server
We execute the program with `go run liveserver.go` (in the *mp_quic/test* dir), which calls `server_wrapper.go` to receive QUIC packets from **AggDeliv Client** and then transforms the packets to RTMP chunks and adds them into the RTMP stream.

***Note***: We should first start **AggDeliv Server** and then run **AggDeliv Client**.

### Demo Video for Test
To test data transmission between **AggDeliv Client** and **AggDeliv Server**, we can push a RTMP stream to **AggDeliv Client** by executing a *FFmpeg* command `ffmpeg -re -i {dir/}demo.flv -c copy -f flv rtmp://localhost:1935/live/{channelkey}` (a demo video *demo.flv* is provided in the *mp_quic* dir). Before executing the command, we need to acquire a *channel key* from **AggDeliv Client** by conducting `HTTP Get http://localhost:8090/control/reset?room=movie` and retrieving the last segment of its response.
