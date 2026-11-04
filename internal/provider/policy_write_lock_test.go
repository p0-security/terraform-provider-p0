package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An in-memory stand-in for P0's policy and bundles APIs that refuses a write
// while another is in flight, the way P0's Firestore transaction on the org's
// one policy document gives up under contention. Bundle writes also write
// their `bundle:<id>` rule, so they contend the same way.
type fakePolicyP0 struct {
	server *httptest.Server

	mu       sync.Mutex
	policies map[string]json.RawMessage
	bundles  map[string]map[string]any
	nextId   int
	writing  bool
	overlaps int
}

func newFakePolicyP0(t *testing.T) *fakePolicyP0 {
	f := &fakePolicyP0{policies: map[string]json.RawMessage{}, bundles: map[string]map[string]any{}}

	mux := http.NewServeMux()
	policy := "/o/" + fakeOrg + "/policy/name/{name}"
	mux.HandleFunc("POST "+policy, f.write)
	mux.HandleFunc("PUT "+policy, f.write)
	mux.HandleFunc("DELETE "+policy, f.write)
	mux.HandleFunc("GET "+policy, f.read)
	bundles := "/o/" + fakeOrg + "/bundles"
	mux.HandleFunc("POST "+bundles, f.writeBundle)
	mux.HandleFunc("PUT "+bundles+"/{id}", f.writeBundle)
	mux.HandleFunc("DELETE "+bundles+"/{id}", f.deleteBundle)
	mux.HandleFunc("GET "+bundles+"/{id}", f.readBundle)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to the fake P0 API: %s %s", r.Method, r.URL.Path)
		notFound(w)
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// beginWrite refuses the request if another write is in flight. Otherwise it
// keeps this one in flight long enough that unserialized writes overlap, and
// returns holding f.mu; the caller must call endWrite.
func (f *fakePolicyP0) beginWrite(w http.ResponseWriter) bool {
	f.mu.Lock()
	if f.writing {
		f.overlaps++
		f.mu.Unlock()
		http.Error(w, `{"error":"10 ABORTED: Too much contention on these documents. Please try again."}`, http.StatusInternalServerError)
		return false
	}
	f.writing = true
	f.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	f.mu.Lock()
	return true
}

func (f *fakePolicyP0) endWrite() {
	f.writing = false
	f.mu.Unlock()
}

func (f *fakePolicyP0) write(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if !f.beginWrite(w) {
		return
	}
	defer f.endWrite()

	name := r.PathValue("name")
	if r.Method == http.MethodDelete {
		delete(f.policies, name)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	f.policies[name] = body
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (f *fakePolicyP0) read(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.policies[r.PathValue("name")]
	if !ok {
		notFound(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(stored)
}

// Five p0_access_policy and five p0_routing_rule resources, all independent, so
// Terraform writes all ten at once at its default parallelism of 10.
func policyWriteLockConfig(f *fakePolicyP0, disabled bool) string {
	var config strings.Builder
	config.WriteString(providerConfig(&fakeP0{server: f.server}))
	for _, kind := range []string{"p0_access_policy", "p0_routing_rule"} {
		for i := range 5 {
			fmt.Fprintf(&config, `
resource %q "policy_%d" {
  name      = "%s-%d"
  disabled  = %t
  requestor = { type = "any" }
  resource  = { type = "any" }
  approval  = [{ type = "p0" }]
}
`, kind, i, kind, i, disabled)
		}
	}
	return config.String()
}

func checkNoOverlappingPolicyWrites(f *fakePolicyP0) func(*terraform.State) error {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.overlaps > 0 {
			return fmt.Errorf("P0 received %d policy writes while another was in flight", f.overlaps)
		}
		return nil
	}
}

// TestPolicyWritesAreSerialized checks that the policy resources of one apply
// write to P0 one at a time, on create, update and destroy.
func TestPolicyWritesAreSerialized(t *testing.T) {
	f := newFakePolicyP0(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := checkNoOverlappingPolicyWrites(f)(s); err != nil {
				return err
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.policies) > 0 {
				return fmt.Errorf("%d policies were not removed from P0", len(f.policies))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: policyWriteLockConfig(f, false),
				Check:  checkNoOverlappingPolicyWrites(f),
			},
			{
				Config: policyWriteLockConfig(f, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkNoOverlappingPolicyWrites(f),
					resource.TestCheckResourceAttr("p0_access_policy.policy_0", "disabled", "true"),
					resource.TestCheckResourceAttr("p0_routing_rule.policy_0", "disabled", "true"),
				),
			},
		},
	})
}
