package postgresql

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
)

// pgTableCacheServer implements startup, Ping and simple queries so collector
// tests use utils.ConnectionDatabase and the real PostgreSQL driver unchanged.
type pgTableCacheServer struct {
	listener net.Listener
	fake     *pgTableCacheSQL
	wg       sync.WaitGroup
	mu       sync.Mutex
	clients  map[net.Conn]bool
	closing  bool
	opened   []string
	closed   int
	changed  chan struct{}
}

func newPGTableCacheServer(t *testing.T, cfg *config.Config, fake *pgTableCacheSQL) *pgTableCacheServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &pgTableCacheServer{
		listener: listener, fake: fake, clients: make(map[net.Conn]bool), changed: make(chan struct{}, 1),
	}
	cfg.PgHost = "127.0.0.1"
	cfg.PgPort = fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	cfg.PgUser, cfg.PgPassword = "test", "test"
	cfg.PgSslMode = false
	t.Cleanup(func() {
		listener.Close()
		server.mu.Lock()
		server.closing = true
		for client := range server.clients {
			client.Close()
		}
		server.mu.Unlock()
		server.wg.Wait()
	})
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server.mu.Lock()
			if server.closing {
				server.mu.Unlock()
				client.Close()
				return
			}
			server.clients[client] = true
			server.mu.Unlock()
			server.wg.Add(1)
			go server.serve(client)
		}
	}()
	return server
}

func (s *pgTableCacheServer) openedDatabases() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.opened)
}

func (s *pgTableCacheServer) waitForClosed(t *testing.T, want int) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed == want {
			return
		}
		select {
		case <-s.changed:
		case <-timer.C:
			t.Fatalf("closed connections = %d, want %d", closed, want)
		}
	}
}

func (s *pgTableCacheServer) serve(client net.Conn) {
	defer s.wg.Done()
	defer func() {
		client.Close()
		s.mu.Lock()
		delete(s.clients, client)
		s.closed++
		s.mu.Unlock()
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}()
	client.SetDeadline(time.Now().Add(5 * time.Second))
	startup, err := readPGTestPayload(client)
	if err != nil || len(startup) < 4 {
		return
	}
	var database string
	fields := strings.Split(string(startup[4:]), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "database" {
			database = fields[i+1]
		}
	}
	s.mu.Lock()
	s.opened = append(s.opened, database)
	s.mu.Unlock()
	reply := pgTestMessage('R', []byte{0, 0, 0, 0}) // AuthenticationOk.
	reply = append(reply, pgTestMessage('S', []byte("server_version\x0016.0\x00"))...)
	reply = append(reply, pgTestMessage('Z', []byte{'I'})...)
	if _, err := client.Write(reply); err != nil {
		return
	}
	for {
		var kind [1]byte
		if _, err := io.ReadFull(client, kind[:]); err != nil {
			return
		}
		payload, err := readPGTestPayload(client)
		if err != nil || kind[0] == 'X' { // Terminate.
			return
		}
		if kind[0] != 'Q' {
			return
		}
		query := strings.TrimSuffix(string(payload), "\x00")
		if query == ";" { // The lib/pq Ping query.
			reply = pgTestMessage('I', nil) // EmptyQueryResponse.
		} else {
			rows, err := s.fake.query(database, query)
			if err != nil {
				reply = pgTestMessage('E', []byte("SERROR\x00CXX000\x00M"+err.Error()+"\x00\x00"))
			} else {
				reply = pgTestRows(rows)
			}
		}
		reply = append(reply, pgTestMessage('Z', []byte{'I'})...)
		if _, err := client.Write(reply); err != nil {
			return
		}
	}
}

func readPGTestPayload(reader io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length < 4 || length > 1<<20 {
		return nil, fmt.Errorf("invalid PostgreSQL message length %d", length)
	}
	payload := make([]byte, length-4)
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

func pgTestMessage(kind byte, payload []byte) []byte {
	message := binary.BigEndian.AppendUint32([]byte{kind}, uint32(len(payload)+4))
	return append(message, payload...)
}

func pgTestRows(rows driver.Rows) []byte {
	defer rows.Close()
	columns := rows.Columns()
	description := binary.BigEndian.AppendUint16(nil, uint16(len(columns)))
	for _, name := range columns {
		description = append(description, name...)
		description = append(description, 0)
		description = binary.BigEndian.AppendUint32(description, 0)  // Table OID.
		description = binary.BigEndian.AppendUint16(description, 0)  // Column number.
		description = binary.BigEndian.AppendUint32(description, 25) // Text OID.
		description = binary.BigEndian.AppendUint16(description, 0xffff)
		description = binary.BigEndian.AppendUint32(description, 0xffffffff)
		description = binary.BigEndian.AppendUint16(description, 0) // Text format.
	}
	result := pgTestMessage('T', description)
	values := make([]driver.Value, len(columns))
	count := 0
	for rows.Next(values) == nil {
		data := binary.BigEndian.AppendUint16(nil, uint16(len(values)))
		for _, value := range values {
			if value == nil {
				data = binary.BigEndian.AppendUint32(data, 0xffffffff)
				continue
			}
			text := fmt.Sprint(value)
			data = binary.BigEndian.AppendUint32(data, uint32(len(text)))
			data = append(data, text...)
		}
		result = append(result, pgTestMessage('D', data)...)
		count++
	}
	return append(result, pgTestMessage('C', []byte(fmt.Sprintf("SELECT %d\x00", count)))...)
}
