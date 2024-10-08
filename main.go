package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/fastly/compute-sdk-go/fsthttp"
	"github.com/fastly/compute-sdk-go/x/exp/handoff"
)

func handleTest(w fsthttp.ResponseWriter, r *fsthttp.Request, channel string) {
	switch r.URL.Path {
	case "/test/long-poll":
		gripResponse(w, "text/plain", "response", channel)
	case "/test/stream":
		gripResponse(w, "text/plain", "stream", channel)
	case "/test/sse":
		gripResponse(w, "text/event-stream", "stream", channel)
	case "/test/websocket":
		handleFanoutWs(w, r, channel)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "No such test endpoint\n")
	}
}

func handleFanoutWs(w fsthttp.ResponseWriter, r *fsthttp.Request, channel string) {
	if r.Header.Get("Content-Type") != "application/websocket-events" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Not a Websocket-Over-HTTP request.\n")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error reading request body.\n")
		return
	}
	if bytes.Equal(body[:6], []byte("OPEN\r\n")) {
		w.Header().Add("Content-Type", "application/websocket-events")
		w.Header().Add("Sec-WebSocket-Extensions", "grip; message-prefix=\"\"")
		w.WriteHeader(http.StatusOK)
		resp := string(append([]byte("OPEN\r\n"), wsSub(channel)...))
		fmt.Fprintf(w, resp)
	} else if bytes.Equal(body[:5], []byte("TEXT ")) {
		s := wsText(fmt.Sprintf("You said: %s\n", string(body[6:])))
		fmt.Fprintf(w, string(s))
	}
}

func gripResponse(w fsthttp.ResponseWriter, ctype string, ghold string, channel string) {
	w.Header().Add("Content-Type", ctype)
	w.Header().Add("Grip-Hold", ghold)
	w.Header().Add("Grip-Channel", channel)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}

func wsText(msg string) []byte {
	return []byte(fmt.Sprintf("TEXT %2x\r\n%s\r\n", len(msg), msg))
}

func wsSub(channel string) []byte {
	return wsText(fmt.Sprintf("c:{\"type\":\"subscribe\",\"channel\":\"%s\"}", channel))
}

func main() {
	fmt.Println("FASTLY_SERVICE_VERSION:", os.Getenv("FASTLY_SERVICE_VERSION"))
	fsthttp.ServeFunc(func(ctx context.Context, w fsthttp.ResponseWriter, r *fsthttp.Request) {
		//defer w.Close()
		host := r.Host
		path := r.URL.Path
		addr := r.RemoteAddr

		w.Header().Set("X-Forwarded-For", addr)

		if strings.ToLower(r.URL.Scheme) == "https" {
			w.Header().Set("X-Forwarded-Proto", "https")
		}

		if strings.HasSuffix(host, ".edgecompute.app") && strings.HasPrefix(path, "/test/") {
			if r.Header.Get("Grip-Sig") != "" {
				handleTest(w, r, "test")
				return
			} else {
				handoff.Fanout("self")
				return
			}
		}

		handoff.Fanout("origin")
	})
}
