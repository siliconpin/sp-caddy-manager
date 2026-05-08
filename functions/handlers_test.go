package functions

import "testing"

func TestCaddyDomainEntriesExtractsPortFromNestedSubroute(t *testing.T) {
	route := map[string]interface{}{
		"match": []interface{}{
			map[string]interface{}{"host": []interface{}{"app.example.com"}},
		},
		"handle": []interface{}{
			map[string]interface{}{
				"handler": "subroute",
				"routes": []interface{}{
					map[string]interface{}{
						"handle": []interface{}{
							map[string]interface{}{
								"handler": "reverse_proxy",
								"upstreams": []interface{}{
									map[string]interface{}{"dial": "localhost:8080"},
								},
							},
						},
					},
				},
			},
		},
	}

	entries := caddyDomainEntries(route)
	if len(entries) != 1 {
		t.Fatalf("expected one domain entry, got %#v", entries)
	}
	if entries[0].Domain != "app.example.com" || entries[0].Port != 8080 {
		t.Fatalf("unexpected domain entry: %#v", entries[0])
	}
}

func TestCaddyDomainEntriesExtractsPortFromLaterUpstream(t *testing.T) {
	route := map[string]interface{}{
		"match": []interface{}{
			map[string]interface{}{"host": []interface{}{"app.example.com"}},
		},
		"handle": []interface{}{
			map[string]interface{}{
				"handler": "reverse_proxy",
				"upstreams": []interface{}{
					map[string]interface{}{"dial": "unix//run/backend.sock"},
					map[string]interface{}{"dial": "127.0.0.1:9090"},
				},
			},
		},
	}

	entries := caddyDomainEntries(route)
	if len(entries) != 1 {
		t.Fatalf("expected one domain entry, got %#v", entries)
	}
	if entries[0].Port != 9090 {
		t.Fatalf("expected port 9090, got %#v", entries[0])
	}
}
