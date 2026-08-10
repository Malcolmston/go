package main

import (
	"bufio"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

// smtpServer is a minimal, in-process ESMTP server bound to 127.0.0.1:0. It
// exists purely so the example can drive nodemailer.SMTPTransport and
// nodemailer.Pool for real — no external mail server is involved and nothing
// ever leaves the process.
type smtpServer struct {
	ln   net.Listener
	host string
	port int

	mu       sync.Mutex
	conns    int
	messages int
	mail     string
	rcpt     string
	datas    []string
}

func startSMTPServer() *smtpServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	s := &smtpServer{ln: ln, host: host, port: port}
	go s.serve()
	return s
}

func (s *smtpServer) close() { _ = s.ln.Close() }

func (s *smtpServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns++
		s.mu.Unlock()
		go s.handle(conn)
	}
}

func (s *smtpServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	write := func(line string) {
		_, _ = w.WriteString(line + "\r\n")
		_ = w.Flush()
	}
	write("220 example.invalid ESMTP in-process")

	var data strings.Builder
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if inData {
			if strings.TrimRight(line, "\r\n") == "." {
				inData = false
				s.mu.Lock()
				s.messages++
				s.datas = append(s.datas, data.String())
				s.mu.Unlock()
				data.Reset()
				write("250 2.0.0 OK queued")
				continue
			}
			data.WriteString(line)
			continue
		}
		trimmed := strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-example.invalid greets you")
			write("250-DSN")
			write("250 SIZE 10485760")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mu.Lock()
			s.mail = trimmed
			s.mu.Unlock()
			write("250 2.1.0 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			s.rcpt = trimmed
			s.mu.Unlock()
			write("250 2.1.5 OK")
		case upper == "DATA":
			inData = true
			write("354 End data with <CRLF>.<CRLF>")
		case upper == "RSET":
			write("250 2.0.0 OK")
		case upper == "NOOP":
			write("250 2.0.0 OK")
		case upper == "QUIT":
			write("221 2.0.0 Bye")
			return
		default:
			write("250 2.0.0 OK")
		}
	}
}

func (s *smtpServer) lastMail() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mail
}

func (s *smtpServer) lastRcpt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rcpt
}

func (s *smtpServer) messageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages
}

func (s *smtpServer) connCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}
