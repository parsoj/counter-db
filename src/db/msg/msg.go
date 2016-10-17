/**
Msg.go

basic structures to hold deserialized forms of api requests
*/
package msg

import (
	"bytes"
	"encoding/json"
	"io"
)

type ConfigMsg struct {
	Actors []string `json:"actors"`
}

func BuildConfigMsg(body io.Reader) ConfigMsg {
	var c ConfigMsg
	json.Unmarshal(requestBodyBytes(body), &c)
	return c
}

type AddMsg struct {
	AddValue int `json:"addValue"`
}

func BuildAddMsg(body io.Reader) AddMsg {
	var a AddMsg
	json.Unmarshal(requestBodyBytes(body), &a)
	return a
}

type SyncMsg struct {
	CounterValues []int `json:"CounterValues"`
}

func BuildSyncMsg(body io.Reader) SyncMsg {
	var s SyncMsg
	reqBytes := requestBodyBytes(body)
	json.Unmarshal(reqBytes, &s)
	return s
}

func requestBodyBytes(body io.Reader) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	return []byte(buf.String())
}
