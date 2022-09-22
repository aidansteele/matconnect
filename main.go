package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/marten-seemann/webtransport-go"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) >= 2 {
		fmt.Println("client mode")
		client(os.Args[1])
	} else {
		fmt.Println("server mode")
		server()
	}
}

func myIp() string {
	if len(os.Args) >= 3 {
		return os.Args[2]
	}

	get, _ := http.Get("http://169.254.169.254/latest/meta-data/public-ipv4")
	myIpBytes, _ := io.ReadAll(get.Body)
	return strings.TrimSpace(string(myIpBytes))
}

func tlsConfig() *tls.Config {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "mat.loves.this.song"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	cert, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	return &tls.Config{
		NextProtos: []string{"h3"},
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{cert},
				PrivateKey:  key,
			},
		},
	}
}

func server() {
	port := 8080
	fmt.Printf("My address is %s:%d\n", myIp(), port)

	srv := webtransport.Server{
		H3: http3.Server{
			Handler:   http.DefaultServeMux,
			TLSConfig: tlsConfig(),
		},
	}

	http.HandleFunc("/mat", func(w http.ResponseWriter, r *http.Request) {
		sess, err := srv.Upgrade(w, r)
		if err != nil {
			http.Error(w, "Unable to upgrade", http.StatusUpgradeRequired)
			panic(fmt.Sprintf("%+v", err))
		}

		ctx := context.Background()
		stream, err := sess.AcceptStream(ctx)
		if err != nil {
			panic(fmt.Sprintf("%+v", err))
		}

		go func() {
			scan := bufio.NewScanner(stream)
			for scan.Scan() {
				line := scan.Text()
				fmt.Printf("< %s\n", line)
			}
		}()

		time.Sleep(250 * time.Millisecond)

		for idx := 1; idx < len(VeryImportantData); idx += 2 {
			line := VeryImportantData[idx]
			fmt.Fprintln(stream, line)
			fmt.Printf("> %s\n", line)
			time.Sleep(500 * time.Millisecond)
		}
	})

	udpl, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		panic(fmt.Sprintf("udp listen %+v", err))
	}

	silly := newMatconn(udpl)
	err = srv.Serve(silly)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}

func client(remoteAddr string) {
	sd := &matdialer{ip: netip.MustParseAddr(myIp())}

	wtd := webtransport.Dialer{RoundTripper: &http3.RoundTripper{
		Dial: sd.Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}}

	ctx := context.Background()

	_, sess, err := wtd.Dial(ctx, fmt.Sprintf("https://%s/mat", remoteAddr), http.Header{})
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	stream, err := sess.OpenStreamSync(ctx)
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	go func() {
		scan := bufio.NewScanner(stream)
		for scan.Scan() {
			line := scan.Text()
			fmt.Printf("< %s\n", line)
		}
	}()

	for idx := 0; idx < len(VeryImportantData); idx += 2 {
		line := VeryImportantData[idx]
		fmt.Fprintln(stream, line)
		fmt.Printf("> %s\n", line)
		time.Sleep(500 * time.Millisecond)
	}
}
