package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func toolNames(body []byte) []string {
	out := []string{}
	gjson.GetBytes(body, "tools").ForEach(func(_, t gjson.Result) bool {
		out = append(out, t.Get("name").String())
		return true
	})
	return out
}

func TestSortToolsByName_OrdersLexically(t *testing.T) {
	body := []byte(`{"tools":[{"name":"write"},{"name":"bash"},{"name":"read"}]}`)
	got := sortToolsByName(body)
	require.Equal(t, []string{"bash", "read", "write"}, toolNames(got))
}

// The core cache-stability invariant: the same tool SET sent in different
// ORDERS must serialize to identical bytes after sorting, so the cache prefix
// matches across turns.
func TestSortToolsByName_SameSetDifferentOrderConverges(t *testing.T) {
	a := []byte(`{"tools":[{"name":"write","x":1},{"name":"bash","x":2},{"name":"read","x":3}]}`)
	b := []byte(`{"tools":[{"name":"read","x":3},{"name":"write","x":1},{"name":"bash","x":2}]}`)
	require.Equal(t, string(sortToolsByName(a)), string(sortToolsByName(b)),
		"same tool set in different order must produce identical bytes after sort")
}

func TestSortToolsByName_PreservesToolBodies(t *testing.T) {
	body := []byte(`{"tools":[{"name":"write","input_schema":{"type":"object"}},{"name":"bash","description":"run"}]}`)
	got := sortToolsByName(body)
	require.Equal(t, "run", gjson.GetBytes(got, `tools.0.description`).String())
	require.Equal(t, "object", gjson.GetBytes(got, `tools.1.input_schema.type`).String())
}

func TestSortToolsByName_Noops(t *testing.T) {
	// not an array
	require.Equal(t, `{"tools":"nope"}`, string(sortToolsByName([]byte(`{"tools":"nope"}`))))
	// single element
	single := []byte(`{"tools":[{"name":"bash"}]}`)
	require.Equal(t, string(single), string(sortToolsByName(single)))
	// no tools key
	require.Equal(t, `{"model":"x"}`, string(sortToolsByName([]byte(`{"model":"x"}`))))
}

func TestSortToolsByName_AlreadySortedIsIdempotent(t *testing.T) {
	body := []byte(`{"tools":[{"name":"bash"},{"name":"read"},{"name":"write"}]}`)
	once := sortToolsByName(body)
	twice := sortToolsByName(once)
	require.Equal(t, string(body), string(once), "already-sorted body must be returned unchanged")
	require.Equal(t, string(once), string(twice))
}

// applyThirdPartyToolMimicry must sort BEFORE building the rename map, so that
// the same tool set in different orders yields byte-identical forwarded bodies
// (fake names included). >5 tools triggers the dynamic-rename path where fake
// names encode the array index — the strongest test of sort-before-build.
func TestApplyThirdPartyToolMimicry_OrderIndependentWithDynamicRename(t *testing.T) {
	mk := func(order []string) []byte {
		raw := `{"tools":[`
		for i, n := range order {
			if i > 0 {
				raw += ","
			}
			raw += `{"name":"` + n + `","type":"custom"}`
		}
		raw += `]}`
		return []byte(raw)
	}
	set := []string{"alpha_x", "beta_x", "gamma_x", "delta_x", "epsilon_x", "zeta_x"}
	shuffled := []string{"zeta_x", "alpha_x", "delta_x", "beta_x", "epsilon_x", "gamma_x"}

	bodyA, rwA := applyThirdPartyToolMimicry(mk(set))
	bodyB, rwB := applyThirdPartyToolMimicry(mk(shuffled))

	require.NotNil(t, rwA)
	require.NotNil(t, rwB)
	require.Equal(t, string(bodyA), string(bodyB),
		"same tool set, different order must forward identical bytes (fake names included)")
	require.Equal(t, rwA.Forward, rwB.Forward, "rename map must be order-independent")
}

// The tools[-1] cache breakpoint must land on the LAST tool after sorting.
func TestApplyThirdPartyToolMimicry_BreakpointOnSortedLast(t *testing.T) {
	// <=5 tools => no dynamic rename, just sort + breakpoint
	body := []byte(`{"tools":[{"name":"write"},{"name":"bash"}]}`)
	got, rw := applyThirdPartyToolMimicry(body)
	require.Nil(t, rw)
	require.Equal(t, []string{"bash", "write"}, toolNames(got))
	// breakpoint on sorted last ("write")
	require.Equal(t, "ephemeral", gjson.GetBytes(got, `tools.1.cache_control.type`).String())
	require.False(t, gjson.GetBytes(got, `tools.0.cache_control`).Exists())
}
