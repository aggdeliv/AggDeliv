package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//const dir = "E:/GolandProjects/mpquic_trans/"

func main() {
	resp, err := http.Get("http://localhost:8090/control/reset?room=movie")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	res := string(body)
	pos_tail := strings.LastIndex(res, "\"")
	pos_head := strings.LastIndex(res[:pos_tail], "\"")
	key := res[pos_head+1 : pos_tail]
	fmt.Println(key)

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	println(dir)
	//dir, _ := os.Getwd()
	//dir, _ := os.Open("F:\\ffmpeg-4.3.1-win64-static\\bin")
	//dir.Chdir()

	comStr := "ffmpeg -re -i demo.flv -c copy -f flv rtmp://localhost:1935/live/" + key
	command := exec.Command(comStr)
	output, err := command.CombinedOutput()
	if err != nil {
		log.Error("device panic: ", err)
	} else {
		fmt.Println(string(output))
	}
}
