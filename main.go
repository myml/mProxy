// mProxy project main.go
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"time"
)

var connChannel chan net.Conn

func GoogleDNSDialer(ctx context.Context, network, address string) (net.Conn, error) {
	for _, server := range []string{"8.8.8.8:53", "8.8.4.4:53"} {
		conn, err := net.Dial("udp", server)
		if err != nil {
			continue
		}
		return conn, err
	}
	return nil, errors.New("dns not available")
}
func CloudflareDnsDialer(ctx context.Context, network, address string) (net.Conn, error) {
	tlsCfg := &tls.Config{
		ServerName: "cloudflare-dns.com",
	}
	for _, server := range []string{"1.1.1.1:853", "1.0.0.1:853"} {
		conn, err := net.Dial("tcp", server)
		if err != nil {
			continue
		}
		setKeepAlive(conn)
		return tls.Client(conn, tlsCfg), nil
	}
	log.Println("dns over tls not available")
	return GoogleDNSDialer(ctx, network, address)
}

func setKeepAlive(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(time.Minute * 3)
		tcp.SetNoDelay(false)
	}
}

func init() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	connChannel = make(chan net.Conn, 5)
}
func connPool() {
	r := net.Resolver{
		PreferGo: true,
		Dial:     CloudflareDnsDialer,
	}
	ips, err := r.LookupIPAddr(context.Background(), *host)
	if err != nil {
		log.Panic(err)
	}
	for {
		ip := ips[rand.Intn(len(ips))]
		s, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip.String(), *port), time.Second*5)
		if err != nil {
			log.Println(err)
			time.Sleep(time.Second)
			continue
		}
		setKeepAlive(s)
		connChannel <- s
	}
}

var host *string
var port *int

func main() {
	host = flag.String("h", "", "remote host")
	port = flag.Int("p", 0, "remote port")
	lport := flag.Int("l", 0, "listen port")
	flag.Parse()

	if *host == "" || *port == 0 {
		log.Println("host and port is required")
		return
	}
	if *lport == 0 {
		lport = port
	}
	log.Printf("listen at port %d,proxy to %s:%d", *lport, *host, *port)

	go connPool()
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", *lport))
	if err != nil {
		log.Panic(err)
	}
	for {
		c, err := l.Accept()
		if err != nil {
			log.Panic(err)
		}
		go proxy(c)
	}
}

func proxy(c net.Conn) {
	s := <-connChannel
	log.Println(s.RemoteAddr())
	defer c.Close()
	defer s.Close()
	go io.Copy(c, s)
	io.Copy(s, c)
}
