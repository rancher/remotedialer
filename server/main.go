package main

import (
        "bufio"
        "context"
        "crypto/tls"
        "flag"
        "fmt"
        "io"
        "net/http"
        "strings"
        "sync/atomic"
        "time"

        "github.com/rancher/remotedialer"
        "github.com/sirupsen/logrus"
)

var counter int64

func authorizer(req *http.Request) (string, bool, error) {
        id := req.Header.Get("x-tunnel-id")
        return id, id != "", nil
}

// proxyHTTP 处理普通 HTTP 请求（支持任意方法）
func proxyHTTP(server *remotedialer.Server, rw http.ResponseWriter, req *http.Request) {
        id := atomic.AddInt64(&counter, 1)

        clientKey := req.PathValue("id")
        scheme := req.PathValue("scheme")
        host := req.PathValue("host")
        path := req.PathValue("path")

        targetURL := fmt.Sprintf("%s://%s", scheme, host)
        if path != "" {
                targetURL += "/" + path
        }
        if req.URL.RawQuery != "" {
                targetURL += "?" + req.URL.RawQuery
        }

        targetAddr := host
        if !strings.Contains(targetAddr, ":") {
                if scheme == "https" {
                        targetAddr += ":443"
                } else {
                        targetAddr += ":80"
                }
        }

        dialer := server.Dialer(clientKey)
        ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
        defer cancel()

        conn, err := dialer(ctx, "tcp", targetAddr)
        if err != nil {
                logrus.Errorf("[%03d] DIAL ERR %s: %v", id, targetAddr, err)
                remotedialer.DefaultErrorWriter(rw, req, 502, err)
                return
        }

        // 新增：根据 scheme 决定是否进行 TLS 握手
        if scheme == "https" {
            tlsConn := tls.Client(conn, &tls.Config{
                ServerName:         host,              // 用于 SNI 和证书校验
                InsecureSkipVerify: true,              // 测试环境跳过校验；生产应加载 CA
            })
            if err := tlsConn.HandshakeContext(ctx); err != nil {
                logrus.Errorf("[%03d] TLS HANDSHAKE ERR %s: %v", id, targetAddr, err)
                conn.Close()
                remotedialer.DefaultErrorWriter(rw, req, 502, err)
                return
            }
            conn = tlsConn  // 后续 Write/Read 都走 TLS 连接
        }
        defer conn.Close()

        // 构造发往目标服务的请求：路径、Host 都要改成目标服务认识的形式
        outReq := req.Clone(req.Context())
        outReq.URL.Scheme = ""
        outReq.URL.Host = ""
        outReq.RequestURI = ""

        // 关键修复：去掉 /client/{id}/{scheme}/{host} 前缀，只保留目标路径
        if path == "" {
                outReq.URL.Path = "/"
        } else {
                outReq.URL.Path = "/" + path
        }
        outReq.URL.RawPath = ""

        // Host 头改成目标服务的 Host
        outReq.Host = host

        removeHopByHopHeaders(outReq.Header)

        if err := outReq.Write(conn); err != nil {
                logrus.Errorf("[%03d] WRITE ERR %s: %v", id, targetURL, err)
                remotedialer.DefaultErrorWriter(rw, req, 502, err)
                return
        }

        resp, err := http.ReadResponse(bufio.NewReader(conn), outReq)
        if err != nil {
                logrus.Errorf("[%03d] READ ERR %s: %v", id, targetURL, err)
                remotedialer.DefaultErrorWriter(rw, req, 502, err)
                return
        }
        defer resp.Body.Close()

        for k, v := range resp.Header {
                for _, h := range v {
                        rw.Header().Add(k, h)
                }
        }
        rw.WriteHeader(resp.StatusCode)

        n, _ := io.Copy(rw, resp.Body)
        logrus.Infof("[%03d] %s %s -> %d (%d bytes)", id, req.Method, targetURL, resp.StatusCode, n)
}

// proxyWebSocket 处理 WebSocket 升级请求
func proxyWebSocket(server *remotedialer.Server, rw http.ResponseWriter, req *http.Request) {
        id := atomic.AddInt64(&counter, 1)

        clientKey := req.PathValue("id")
        scheme := req.PathValue("scheme")
        host := req.PathValue("host")
        path := req.PathValue("path")

        targetAddr := host
        if !strings.Contains(targetAddr, ":") {
                if scheme == "wss" || scheme == "https" {
                        targetAddr += ":443"
                } else {
                        targetAddr += ":80"
                }
        }

        dialer := server.Dialer(clientKey)
        ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
        defer cancel()

        conn, err := dialer(ctx, "tcp", targetAddr)
        if err != nil {
                logrus.Errorf("[%03d] WS DIAL ERR %s: %v", id, targetAddr, err)
                http.Error(rw, "dial failed: "+err.Error(), http.StatusBadGateway)
                return
        }

        if scheme == "wss" {
            tlsConn := tls.Client(conn, &tls.Config{
                ServerName:         host,
                InsecureSkipVerify: true,
            })
    
            if err := tlsConn.HandshakeContext(ctx); err != nil {
                logrus.Errorf("[%03d] WS TLS HANDSHAKE ERR %s: %v", id, targetAddr, err)
                conn.Close()
                http.Error(rw, "tls handshake failed: "+err.Error(), http.StatusBadGateway)
                return
            }  

            conn = tlsConn
        }

        defer conn.Close()

        // 构造发往后端的 WebSocket 握手请求
        outReq := req.Clone(req.Context())
        outReq.URL.Scheme = ""
        outReq.URL.Host = ""
        outReq.RequestURI = ""

        if path == "" {
                outReq.URL.Path = "/"
        } else {
                outReq.URL.Path = "/" + path
        }
        outReq.URL.RawPath = ""
        outReq.Host = host

        // 注意：WebSocket 握手必须保留 Upgrade / Connection 头，所以不能调用 removeHopByHopHeaders
        // 这里只移除代理相关的头
        outReq.Header.Del("Proxy-Authenticate")
        outReq.Header.Del("Proxy-Authorization")

        if err := outReq.Write(conn); err != nil {
                logrus.Errorf("[%03d] WS WRITE ERR: %v", id, err)
                http.Error(rw, "write failed", http.StatusBadGateway)
                return
        }

        br := bufio.NewReader(conn)
        resp, err := http.ReadResponse(br, outReq)
        if err != nil {
                logrus.Errorf("[%03d] WS READ ERR: %v", id, err)
                http.Error(rw, "read failed", http.StatusBadGateway)
                return
        }

        if resp.StatusCode != http.StatusSwitchingProtocols {
                for k, v := range resp.Header {
                        for _, h := range v {
                                rw.Header().Add(k, h)
                        }
                }
                rw.WriteHeader(resp.StatusCode)
                io.Copy(rw, resp.Body)
                resp.Body.Close()
                return
        }

        hijacker, ok := rw.(http.Hijacker)
        if !ok {
                http.Error(rw, "hijacking not supported", http.StatusInternalServerError)
                return
        }
        clientConn, clientBuf, err := hijacker.Hijack()
        if err != nil {
                logrus.Errorf("[%03d] WS HIJACK ERR: %v", id, err)
                return
        }
        defer clientConn.Close()

        if err := resp.Write(clientConn); err != nil {
                logrus.Errorf("[%03d] WS WRITE 101 ERR: %v", id, err)
                return
        }

        if br.Buffered() > 0 {
                buffered, _ := br.Peek(br.Buffered())
                clientConn.Write(buffered)
                br.Discard(len(buffered))
        }
        if clientBuf.Reader.Buffered() > 0 {
                buffered, _ := clientBuf.Reader.Peek(clientBuf.Reader.Buffered())
                conn.Write(buffered)
                clientBuf.Reader.Discard(len(buffered))
        }

        logrus.Infof("[%03d] WS UPGRADED %s/%s/%s", id, scheme, host, path)

        done := make(chan struct{}, 2)
        go func() {
                io.Copy(conn, clientConn)
                conn.Close()
                done <- struct{}{}
        }()
        go func() {
                io.Copy(clientConn, conn)
                clientConn.Close()
                done <- struct{}{}
        }()
        <-done
        logrus.Infof("[%03d] WS CLOSED", id)
}

func isWebSocketRequest(req *http.Request) bool {
        return strings.EqualFold(req.Header.Get("Upgrade"), "websocket") &&
                strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")
}

// removeHopByHopHeaders 移除普通 HTTP 转发中不应传递的 hop-by-hop header
func removeHopByHopHeaders(h http.Header) {
        for _, k := range []string{
                "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
                "Te", "Trailer", "Transfer-Encoding", "Upgrade",
        } {
                h.Del(k)
        }
}

func dispatch(server *remotedialer.Server, rw http.ResponseWriter, req *http.Request) {
        if isWebSocketRequest(req) {
                proxyWebSocket(server, rw, req)
                return
        }
        proxyHTTP(server, rw, req)
}

func main() {
        var (
                addr      string
                rawAddr   string
                peerID    string
                peerToken string
                peers     string
                debug     bool
        )
        flag.StringVar(&addr, "listen", ":8123", "Listen address")
        flag.StringVar(&rawAddr, "socks5-listen", ":2222", "SOCKS5 listen address for SSH/any TCP")
        flag.StringVar(&peerID, "id", "", "Peer ID")
        flag.StringVar(&peerToken, "token", "", "Peer Token")
        flag.StringVar(&peers, "peers", "", "Peers format id:token:url,id:token:url")
        flag.BoolVar(&debug, "debug", false, "Enable debug logging")
        flag.Parse()

        if debug {
                logrus.SetLevel(logrus.DebugLevel)
                remotedialer.PrintTunnelData = true
        }

        handler := remotedialer.New(authorizer, remotedialer.DefaultErrorWriter)
        handler.PeerToken = peerToken
        handler.PeerID = peerID

        for _, peer := range strings.Split(peers, ",") {
                parts := strings.SplitN(strings.TrimSpace(peer), ":", 3)
                if len(parts) != 3 {
                        continue
                }
                handler.AddPeer(parts[2], parts[0], parts[1])
        }

        // 启动 SOCKS5 监听（和 HTTP 监听并行，共用同一个 handler）
        if rawAddr != "" {
                if err := startSocks5Listener(handler, rawAddr); err != nil {
                        logrus.Fatalf("start socks5 listener failed: %v", err)
                }
        }

        router := http.NewServeMux()
        router.Handle("/connect", handler)

        clientHandler := func(rw http.ResponseWriter, req *http.Request) {
                dispatch(handler, rw, req)
        }
        router.HandleFunc("/client/{id}/{scheme}/{host}", clientHandler)
        router.HandleFunc("/client/{id}/{scheme}/{host}/{path...}", clientHandler)

        fmt.Println("Listening on ", addr)
        http.ListenAndServe(addr, router)
}
