// Need for unit and integration tests

package logstash

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/urso/ucfg"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/common/streambuf"
	"github.com/elastic/beats/libbeat/outputs"
)

const (
	logstashDefaultHost     = "localhost"
	logstashTestDefaultPort = "5044"
)

type mockLSServer struct {
	*mockServer
}

var testOptions = outputs.Options{}

func newMockTLSServer(t *testing.T, to time.Duration, cert string) *mockLSServer {
	return &mockLSServer{newMockServerTLS(t, to, cert, nil)}
}

func newMockTCPServer(t *testing.T, to time.Duration) *mockLSServer {
	return &mockLSServer{newMockServerTCP(t, to, "", nil)}
}

func (m *mockLSServer) readMessage(buf *streambuf.Buffer, client net.Conn) *message {
	if m.err != nil {
		return nil
	}

	m.clientDeadline(client, m.timeout)
	if m.err != nil {
		return nil
	}

	msg, err := sockReadMessage(buf, client)
	m.err = err
	return msg
}

func (m *mockLSServer) sendACK(client net.Conn, seq uint32) {
	if m.err == nil {
		m.err = sockSendACK(client, seq)
	}
}

func strDefault(a, defaults string) string {
	if len(a) == 0 {
		return defaults
	}
	return a
}

func getenv(name, defaultValue string) string {
	return strDefault(os.Getenv(name), defaultValue)
}

func getLogstashHost() string {
	return fmt.Sprintf("%v:%v",
		getenv("LS_HOST", logstashDefaultHost),
		getenv("LS_TCP_PORT", logstashTestDefaultPort),
	)
}

func testEvent() common.MapStr {
	return common.MapStr{
		"@timestamp": common.Time(time.Now()),
		"type":       "log",
		"extra":      10,
		"message":    "message",
	}
}

func testLogstashIndex(test string) string {
	return fmt.Sprintf("beat-logstash-int-%v-%d", test, os.Getpid())
}

func newTestLumberjackOutput(
	t *testing.T,
	test string,
	config map[string]interface{},
) outputs.BulkOutputer {
	if config == nil {
		config = map[string]interface{}{
			"hosts": []string{getLogstashHost()},
			"index": testLogstashIndex(test),
		}
	}

	plugin := outputs.FindOutputPlugin("logstash")
	if plugin == nil {
		t.Fatalf("No logstash output plugin found")
	}

	cfg, _ := ucfg.NewFrom(config, ucfg.PathSep("."))
	output, err := plugin(cfg, 0)
	if err != nil {
		t.Fatalf("init logstash output plugin failed: %v", err)
	}

	return output.(outputs.BulkOutputer)
}

func testOutputerFactory(
	t *testing.T,
	test string,
	config map[string]interface{},
) func() outputs.BulkOutputer {
	return func() outputs.BulkOutputer {
		return newTestLumberjackOutput(t, test, config)
	}
}

func sockReadMessage(buf *streambuf.Buffer, in io.Reader) (*message, error) {
	for {
		// try parse message from buffered data
		msg, err := readMessage(buf)
		if msg != nil || (err != nil && err != streambuf.ErrNoMoreBytes) {
			return msg, err
		}

		// read next bytes from socket if incomplete message in buffer
		buffer := make([]byte, 1024)
		n, err := in.Read(buffer)
		if err != nil {
			return nil, err
		}

		buf.Write(buffer[:n])
	}
}

func sockSendACK(out io.Writer, seq uint32) error {
	buf := streambuf.New(nil)
	buf.WriteByte('2')
	buf.WriteByte('A')
	buf.WriteNetUint32(seq)
	_, err := out.Write(buf.Bytes())
	return err
}

// genCertsIfMIssing generates a testing certificate for ip 127.0.0.1 for
// testing if certificate or key is missing. Generated is used for CA,
// client-auth and server-auth. Use only for testing.
func genCertsForIPIfMIssing(
	t *testing.T,
	ip net.IP,
	name string,
) error {
	capem := name + ".pem"
	cakey := name + ".key"

	_, err := os.Stat(capem)
	if err == nil {
		_, err = os.Stat(cakey)
		if err == nil {
			return nil
		}
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		t.Fatalf("failed to generate serial number: %s", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:            []string{"US"},
			Organization:       []string{"elastic"},
			OrganizationalUnit: []string{"beats"},
		},
		Issuer: pkix.Name{
			Country:            []string{"US"},
			Organization:       []string{"elastic"},
			OrganizationalUnit: []string{"beats"},
			Locality:           []string{"locality"},
			Province:           []string{"province"},
			StreetAddress:      []string{"Mainstreet"},
			PostalCode:         []string{"12345"},
			SerialNumber:       "23",
			CommonName:         "*",
		},
		IPAddresses: []net.IP{ip},

		SignatureAlgorithm:    x509.SHA512WithRSA,
		PublicKeyAlgorithm:    x509.ECDSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SubjectKeyId:          []byte("12345"),
		BasicConstraintsValid: true,
		IsCA: true,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign,
	}

	// generate keys
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("failed to generate ca private key: %v", err)
	}
	pub := &priv.PublicKey

	// generate certificate
	caBytes, err := x509.CreateCertificate(
		rand.Reader,
		&caTemplate,
		&caTemplate,
		pub, priv)
	if err != nil {
		t.Fatalf("failed to generate ca certificate: %v", err)
	}

	// write key file
	keyOut, err := os.OpenFile(cakey, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("failed to open key file for writing: %v", err)
	}
	pem.Encode(keyOut, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()

	// write certificate
	certOut, err := os.Create(capem)
	if err != nil {
		t.Fatalf("failed to open cert.pem for writing: %s", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	certOut.Close()

	return nil
}

func TestLogstashTCP(t *testing.T) {
	timeout := 2 * time.Second
	server := newMockTCPServer(t, timeout)

	// create lumberjack output client
	config := map[string]interface{}{
		"hosts":   []string{server.Addr()},
		"timeout": 2,
	}
	testConnectionType(t, server, testOutputerFactory(t, "", config))
}

func TestLogstashTLS(t *testing.T) {
	certName := "ca_test"
	ip := net.IP{127, 0, 0, 1}

	timeout := 2 * time.Second
	genCertsForIPIfMIssing(t, ip, certName)
	server := newMockTLSServer(t, timeout, certName)

	config := map[string]interface{}{
		"hosts":                       []string{server.Addr()},
		"timeout":                     2,
		"tls.certificate_authorities": []string{certName + ".pem"},
	}
	testConnectionType(t, server, testOutputerFactory(t, "", config))
}

func TestLogstashInvalidTLSInsecure(t *testing.T) {
	certName := "ca_invalid_test"
	ip := net.IP{1, 2, 3, 4}

	timeout := 2 * time.Second
	genCertsForIPIfMIssing(t, ip, certName)
	server := newMockTLSServer(t, timeout, certName)

	config := map[string]interface{}{
		"hosts":                       []string{server.Addr()},
		"timeout":                     2,
		"max_retries":                 1,
		"tls.insecure":                true,
		"tls.certificate_authorities": []string{certName + ".pem"},
	}
	testConnectionType(t, server, testOutputerFactory(t, "", config))
}

func testConnectionType(
	t *testing.T,
	server *mockLSServer,
	makeOutputer func() outputs.BulkOutputer,
) {
	var result struct {
		err       error
		win, data *message
		signal    bool
	}

	var wg struct {
		ready  sync.WaitGroup
		finish sync.WaitGroup
	}

	wg.ready.Add(1)  // server signaling readiness to client worker
	wg.finish.Add(2) // server/client signaling test end

	// server loop
	go func() {
		defer wg.finish.Done()
		wg.ready.Done()

		client := server.accept()
		server.handshake(client)

		buf := streambuf.New(nil)
		result.win = server.readMessage(buf, client)
		result.data = server.readMessage(buf, client)
		server.sendACK(client, 1)
		result.err = server.err
	}()

	// worker loop
	go func() {
		defer wg.finish.Done()
		wg.ready.Wait()

		output := makeOutputer()

		signal := outputs.NewSyncSignal()
		output.PublishEvent(signal, testOptions, testEvent())
		result.signal = signal.Wait()
	}()

	// wait shutdown
	wg.finish.Wait()
	server.Close()

	// validate output
	assert.Nil(t, result.err)
	assert.True(t, result.signal)

	data := result.data
	assert.NotNil(t, result.win)
	assert.NotNil(t, result.data)
	if data != nil {
		assert.Equal(t, 1, len(data.events))
		data = data.events[0]
		assert.Equal(t, 10.0, data.doc["extra"])
		assert.Equal(t, "message", data.doc["message"])
	}

}

func TestLogstashInvalidTLS(t *testing.T) {
	certName := "ca_invalid_test"
	ip := net.IP{1, 2, 3, 4}

	timeout := 2 * time.Second
	genCertsForIPIfMIssing(t, ip, certName)
	server := newMockTLSServer(t, timeout, certName)

	config := map[string]interface{}{
		"hosts":                       []string{server.Addr()},
		"timeout":                     1,
		"max_retries":                 0,
		"tls.certificate_authorities": []string{certName + ".pem"},
	}

	var result struct {
		err           error
		handshakeFail bool
		signal        bool
	}

	var wg struct {
		ready  sync.WaitGroup
		finish sync.WaitGroup
	}

	wg.ready.Add(1)  // server signaling readiness to client worker
	wg.finish.Add(2) // server/client signaling test end

	// server loop
	go func() {
		defer wg.finish.Done()
		wg.ready.Done()

		client := server.accept()
		if server.err != nil {
			t.Fatalf("server error: %v", server.err)
		}

		server.handshake(client)
		result.handshakeFail = server.err != nil
	}()

	// client loop
	go func() {
		defer wg.finish.Done()
		wg.ready.Wait()

		output := newTestLumberjackOutput(t, "", config)

		signal := outputs.NewSyncSignal()
		output.PublishEvent(signal, testOptions, testEvent())
		result.signal = signal.Wait()
	}()

	// wait shutdown
	wg.finish.Wait()
	server.Close()

	// validate output
	assert.True(t, result.handshakeFail)
	assert.False(t, result.signal)
}
