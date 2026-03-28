package chromecast

// DashCast support — opens a web page URL on a Chromecast using the DashCast
// receiver app (app ID 84912283). This bypasses the go-chromecast library's
// unexported internals by talking the Cast protocol directly over TLS.
//
// Protocol flow:
//  1. TLS connect to device:port
//  2. CONNECT on transport namespace (sender-0 → receiver-0)
//  3. LAUNCH DashCast app on receiver namespace
//  4. Wait for RECEIVER_STATUS with DashCast sessionId
//  5. CONNECT on transport namespace (sender-0 → sessionId)
//  6. Send {"url":..., "force":true} on DashCast namespace

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/gogo/protobuf/proto"

	pb "github.com/vishen/go-chromecast/cast/proto"
)

const (
	dashcastAppID    = "84912283"
	dashcastNS       = "urn:x-cast:com.madmod.dashcast"
	transportNS      = "urn:x-cast:com.google.cast.tp.connection"
	receiverNS       = "urn:x-cast:com.google.cast.receiver"
	senderID         = "sender-0"
	receiverID       = "receiver-0"
)

func castSite(addr string, port int, url string) error {
	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", addr, port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	send := func(srcID, dstID, ns string, payload interface{}) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		s := string(data)
		msg := &pb.CastMessage{
			ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
			SourceId:        &srcID,
			DestinationId:   &dstID,
			Namespace:       &ns,
			PayloadType:     pb.CastMessage_STRING.Enum(),
			PayloadUtf8:     &s,
		}
		b, err := proto.Marshal(msg)
		if err != nil {
			return err
		}
		if err := binary.Write(conn, binary.BigEndian, uint32(len(b))); err != nil {
			return err
		}
		_, err = conn.Write(b)
		return err
	}

	recv := func() (*pb.CastMessage, error) {
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return nil, err
		}
		msg := &pb.CastMessage{}
		return msg, proto.Unmarshal(buf, msg)
	}

	// Step 1: connect to receiver
	if err := send(senderID, receiverID, transportNS, map[string]string{"type": "CONNECT"}); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// Step 2: launch DashCast
	if err := send(senderID, receiverID, receiverNS, map[string]interface{}{
		"type":      "LAUNCH",
		"appId":     dashcastAppID,
		"requestId": 1,
	}); err != nil {
		return fmt.Errorf("launch: %w", err)
	}

	// Step 3: wait for RECEIVER_STATUS with DashCast session
	sessionID := ""
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := recv()
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		var status struct {
			Type   string `json:"type"`
			Status struct {
				Applications []struct {
					AppId     string `json:"appId"`
					SessionId string `json:"sessionId"`
				} `json:"applications"`
			} `json:"status"`
		}
		if err := json.Unmarshal([]byte(msg.GetPayloadUtf8()), &status); err != nil {
			continue
		}
		if status.Type == "RECEIVER_STATUS" {
			for _, app := range status.Status.Applications {
				if app.AppId == dashcastAppID {
					sessionID = app.SessionId
					break
				}
			}
		}
		if sessionID != "" {
			break
		}
	}
	if sessionID == "" {
		return fmt.Errorf("DashCast app did not launch in time")
	}
	log.Printf("DashCast session: %s", sessionID)

	// Step 4: connect to the DashCast session transport
	if err := send(senderID, sessionID, transportNS, map[string]string{"type": "CONNECT"}); err != nil {
		return fmt.Errorf("connect session: %w", err)
	}

	// Step 5: send URL
	return send(senderID, sessionID, dashcastNS, map[string]interface{}{
		"url":   url,
		"force": true,
	})
}
