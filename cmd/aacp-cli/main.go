package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

type broadcastPayload struct {
	Txs    [][]byte `json:"txs"`
}

func main() {
	var (
		nodeURL = flag.String("node", "http://127.0.0.1:8888", "aacpd endpoint")
		cmd = flag.String("cmd", "health", "command: health|finalize-empty|send")
		txHex = flag.String("tx", "", "hex encoded tx bytes for send command")
	)
	flag.Parse()

	switch *cmd {
	case "health":
		get(*nodeURL + "/api/health")
	case "finalize-empty":
		get(*nodeURL + "/api/finalize-empty")
	case "send":
		if *txHex == "" {
			fmt.Fprintln(os.Stderr, "-tx is required")
			os.Exit(1)
		}
		tx, err := hex.DecodeString(*txHex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "decode tx hex failed: %v\n", err)
			os.Exit(1)
		}
		body, _ := json.Marshal(&broadcastPayload{Txs: [][]byte{tx}})
		post(*nodeURL+"/api/finalize", body)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", *cmd)
		os.Exit(1)
	}
}

func get(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Println(string(b))
}

func post(url string, body []byte) {
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Println(string(b))
}
