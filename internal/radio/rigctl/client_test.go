package rigctl

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestPTTUsesOnePersistentConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	gotCommands := make(chan []string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			gotCommands <- nil
			return
		}
		defer conn.Close()
		var commands []string
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			commands = append(commands, scanner.Text())
			_, _ = fmt.Fprintln(conn, "RPRT 0")
		}
		gotCommands <- commands
	}()

	client := New(listener.Addr().String())
	client.Timeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := client.StartPTT(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SetPTT(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-gotCommands:
		if !reflect.DeepEqual(got, []string{"T 1", "T 0"}) {
			t.Fatalf("commands = %#v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for rigctld commands")
	}
}
