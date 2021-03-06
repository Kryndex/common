package server

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/common/httpgrpc"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"
)

type testServer struct {
	*Server
	URL        string
	grpcServer *grpc.Server
}

func newTestServer(handler http.Handler) (*testServer, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	server := &testServer{
		Server:     NewServer(handler),
		grpcServer: grpc.NewServer(),
		URL:        "direct://" + lis.Addr().String(),
	}

	httpgrpc.RegisterHTTPServer(server.grpcServer, server.Server)
	go server.grpcServer.Serve(lis)

	return server, nil
}

func TestBasic(t *testing.T) {
	server, err := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "world")
	}))
	require.NoError(t, err)
	defer server.grpcServer.GracefulStop()

	client, err := NewClient(server.URL)
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "/hello", &bytes.Buffer{})
	require.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "world", string(recorder.Body.Bytes()))
	assert.Equal(t, 200, recorder.Code)
}

func TestError(t *testing.T) {
	server, err := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Does a Fprintln, injecting a newline.
		http.Error(w, "foo", http.StatusInternalServerError)
	}))
	require.NoError(t, err)
	defer server.grpcServer.GracefulStop()

	client, err := NewClient(server.URL)
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "/hello", &bytes.Buffer{})
	require.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "foo\n", string(recorder.Body.Bytes()))
	assert.Equal(t, 500, recorder.Code)
}

func TestParseURL(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
		err      error
	}{
		{"direct://foo", "foo", nil},
		{"kubernetes://foo:123", "kubernetes://foo:123", nil},
		{"querier.cortex:995", "kubernetes://querier:995", nil},
	} {
		got, _, err := ParseURL(tc.input)
		if !reflect.DeepEqual(tc.err, err) {
			t.Fatalf("Got: %v, expected: %v", err, tc.err)
		}
		assert.Equal(t, tc.expected, got)
	}
}
