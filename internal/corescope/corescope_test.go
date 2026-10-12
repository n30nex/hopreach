package corescope

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRegionalNodesPageByRequestedOffsetAndHasMore(t *testing.T) {
	var offsets []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("region") != "YKF" {
			t.Error("missing IATA filter")
		}
		offsets = append(offsets, r.URL.Query().Get("offset"))
		switch r.URL.Query().Get("offset") {
		case "0":
			fmt.Fprint(w, `{"nodes":[],"total":1001,"has_more":true}`)
		case "500":
			fmt.Fprint(w, `{"nodes":[{"public_key":"a"}],"total":1001,"has_more":true}`)
		case "1000":
			fmt.Fprint(w, `{"nodes":[{"public_key":"b"}],"total":1001,"has_more":false}`)
		default:
			t.Fatal("unbounded or duplicate page")
		}
	}))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	c.Region = "YKF"
	nodes, err := c.FetchRepeaters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || !reflect.DeepEqual(offsets, []string{"0", "500", "1000"}) {
		t.Fatalf("nodes=%v offsets=%v", nodes, offsets)
	}
}

func TestNodePagingLegacyAndAllNodes(t *testing.T) {
	for _, all := range []bool{false, true} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"nodes":[{"public_key":"a"}],"total":1}`)
		}))
		c := NewClient(s.URL, s.Client())
		var nodes []Node
		var err error
		if all {
			nodes, err = c.FetchAllNodes(context.Background())
		} else {
			nodes, err = c.FetchRepeaters(context.Background())
		}
		s.Close()
		if err != nil || len(nodes) != 1 {
			t.Fatalf("all=%v nodes=%v err=%v", all, nodes, err)
		}
	}
}
