package main

import (
        "bufio"
        "context"
        "io"
        "net"
        "strconv"
        "sync/atomic"
        "time"

        "github.com/rancher/remotedialer"
        "github.com/sirupsen/logrus"
)

// handleSocks5 处理一条 SOCKS5 连接：
//   - clientKey 从 SOCKS5 用户名（UNAME）拿
//   - target   从 SOCKS5 CONNECT 请求的 ADDR:PORT 拿
//
// 握手成功后，通过 server.Dialer(clientKey) 拨号，然后裸字节双向透传。
func handleSocks5(server *remotedialer.Server, c net.Conn) {
        defer c.Close()

        id := atomic.AddInt64(&counter, 1)

        // 整条握手过程加个超时，防止恶意连接占着不放
        _ = c.SetDeadline(time.Now().Add(30 * time.Second))

        // ---- 阶段 1：方法协商 ----
        // 客户端发: VER(1) NMETHODS(1) METHODS(N)
        header := make([]byte, 2)
        if _, err := io.ReadFull(c, header); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ HEADER ERR: %v", id, err)
                return
        }
        if header[0] != 0x05 {
                logrus.Errorf("[%03d] SOCKS5 BAD VER: %d", id, header[0])
                return
        }
        nMethods := int(header[1])
        methods := make([]byte, nMethods)
        if _, err := io.ReadFull(c, methods); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ METHODS ERR: %v", id, err)
                return
        }
        // 只支持用户名/密码认证（0x02），它承载 clientKey
        supportsUserPass := false
        for _, m := range methods {
                if m == 0x02 {
                        supportsUserPass = true
                        break
                }
        }
        if !supportsUserPass {
                _, _ = c.Write([]byte{0x05, 0xFF}) // 没有可接受的方法
                logrus.Errorf("[%03d] SOCKS5 NO ACCEPTABLE METHOD", id)
                return
        }
        if _, err := c.Write([]byte{0x05, 0x02}); err != nil {
                logrus.Errorf("[%03d] SOCKS5 WRITE METHOD ERR: %v", id, err)
                return
        }

        // ---- 阶段 2：用户名/密码认证（UNAME = clientKey）----
        authHeader := make([]byte, 2)
        if _, err := io.ReadFull(c, authHeader); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ AUTH HEADER ERR: %v", id, err)
                return
        }
        if authHeader[0] != 0x01 {
                logrus.Errorf("[%03d] SOCKS5 BAD AUTH VER: %d", id, authHeader[0])
                return
        }
        ulen := int(authHeader[1])
        uname := make([]byte, ulen)
        if _, err := io.ReadFull(c, uname); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ UNAME ERR: %v", id, err)
                return
        }
        plenBuf := make([]byte, 1)
        if _, err := io.ReadFull(c, plenBuf); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ PLEN ERR: %v", id, err)
                return
        }
        plen := int(plenBuf[0])
        passwd := make([]byte, plen)
        if _, err := io.ReadFull(c, passwd); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ PASSWD ERR: %v", id, err)
                return
        }

        clientKey := string(uname)
        _ = passwd // 这里可以校验 passwd（比如做 token 校验），当前忽略

        // 认证成功
        if _, err := c.Write([]byte{0x01, 0x00}); err != nil {
                logrus.Errorf("[%03d] SOCKS5 WRITE AUTH OK ERR: %v", id, err)
                return
        }

        // ---- 阶段 3：CONNECT 请求（ADDR:PORT = target）----
        reqHeader := make([]byte, 4)
        if _, err := io.ReadFull(c, reqHeader); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ REQ HEADER ERR: %v", id, err)
                return
        }
        if reqHeader[0] != 0x05 || reqHeader[1] != 0x01 {
                logrus.Errorf("[%03d] SOCKS5 BAD REQ: ver=%d cmd=%d", id, reqHeader[0], reqHeader[1])
                // REP=0x07 (Command not supported)
                _, _ = c.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
                return
        }

        var host string
        switch reqHeader[3] { // ATYP
        case 0x01: // IPv4
                addr := make([]byte, 4)
                if _, err := io.ReadFull(c, addr); err != nil {
                        logrus.Errorf("[%03d] SOCKS5 READ IPV4 ERR: %v", id, err)
                        return
                }
                host = net.IP(addr).String()
        case 0x03: // 域名
                l := make([]byte, 1)
                if _, err := io.ReadFull(c, l); err != nil {
                        logrus.Errorf("[%03d] SOCKS5 READ DOMAIN LEN ERR: %v", id, err)
                        return
                }
                domain := make([]byte, int(l[0]))
                if _, err := io.ReadFull(c, domain); err != nil {
                        logrus.Errorf("[%03d] SOCKS5 READ DOMAIN ERR: %v", id, err)
                        return
                }
                host = string(domain)
        case 0x04: // IPv6
                addr := make([]byte, 16)
                if _, err := io.ReadFull(c, addr); err != nil {
                        logrus.Errorf("[%03d] SOCKS5 READ IPV6 ERR: %v", id, err)
                        return
                }
                host = net.IP(addr).String()
        default:
                logrus.Errorf("[%03d] SOCKS5 BAD ATYP: %d", id, reqHeader[3])
                _, _ = c.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Address type not supported
                return
        }
        portBuf := make([]byte, 2)
        if _, err := io.ReadFull(c, portBuf); err != nil {
                logrus.Errorf("[%03d] SOCKS5 READ PORT ERR: %v", id, err)
                return
        }
        port := int(portBuf[0])<<8 | int(portBuf[1])
        targetAddr := net.JoinHostPort(host, strconv.Itoa(port))

        logrus.Infof("[%03d] SOCKS5 client=%s target=%s", id, clientKey, targetAddr)

        // 握手完成，取消 deadline；后续透传不设总超时（由双方自行控制）
        _ = c.SetDeadline(time.Time{})

        // ---- 阶段 4：通过隧道拨号 ----
        dialer := server.Dialer(clientKey)
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        remoteConn, err := dialer(ctx, "tcp", targetAddr)
        if err != nil {
                logrus.Errorf("[%03d] SOCKS5 DIAL ERR %s: %v", id, targetAddr, err)
                // REP=0x05 (Connection refused) —— 这里统一用 0x05 表示拨号失败
                _, _ = c.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
                return
        }
        defer remoteConn.Close()

        // 回 SOCKS5 成功响应：REP=0x00，BND.ADDR/PORT 填 0
        if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
                logrus.Errorf("[%03d] SOCKS5 WRITE OK ERR: %v", id, err)
                return
        }

        // ---- 阶段 5：裸字节双向透传 ----
        done := make(chan struct{}, 2)
        go func() {
                _, _ = io.Copy(remoteConn, c)
                _ = remoteConn.Close()
                done <- struct{}{}
        }()
        go func() {
                _, _ = io.Copy(c, remoteConn)
                _ = c.Close()
                done <- struct{}{}
        }()
        <-done
        logrus.Infof("[%03d] SOCKS5 CLOSED client=%s target=%s", id, clientKey, targetAddr)
}

// startSocks5Listener 起一个 SOCKS5 监听，每条连接交给 handleSocks5。
func startSocks5Listener(server *remotedialer.Server, listenAddr string) error {
        ln, err := net.Listen("tcp", listenAddr)
        if err != nil {
                return err
        }
        logrus.Infof("SOCKS5 listening on %s", listenAddr)
        go func() {
                for {
                        conn, err := ln.Accept()
                        if err != nil {
                                logrus.Errorf("SOCKS5 ACCEPT ERR: %v", err)
                                return
                        }
                        go handleSocks5(server, conn)
                }
        }()
        return nil
}

// 保留 bufio import 的占位（如果你以后想加自定义握手，会用到 bufio）
var _ = bufio.NewReader
