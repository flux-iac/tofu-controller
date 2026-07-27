package main

import (
	"reflect"
	"testing"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBuildCacheOptions(t *testing.T) {
	tests := []struct {
		name                       string
		watchNamespace             string
		additionalSourceNamespaces []string
		wantDefaultNamespaces      map[string]ctrlcache.Config
		wantByObjectNamespaces     map[string]ctrlcache.Config // nil means ByObject must be unset entirely
	}{
		{
			name:                       "watching all namespaces is a no-op regardless of additional namespaces",
			watchNamespace:             "",
			additionalSourceNamespaces: []string{"flux-system"},
			wantDefaultNamespaces:      nil,
			wantByObjectNamespaces:     nil,
		},
		{
			name:                       "namespace-scoped with no additional namespaces matches today's behavior",
			watchNamespace:             "dev",
			additionalSourceNamespaces: nil,
			wantDefaultNamespaces:      map[string]ctrlcache.Config{"dev": {}},
			wantByObjectNamespaces:     nil,
		},
		{
			name:                       "namespace-scoped with additional namespaces sets ByObject for source kinds only",
			watchNamespace:             "dev",
			additionalSourceNamespaces: []string{"flux-system"},
			wantDefaultNamespaces:      map[string]ctrlcache.Config{"dev": {}},
			wantByObjectNamespaces:     map[string]ctrlcache.Config{"dev": {}, "flux-system": {}},
		},
		{
			name:                       "blank and whitespace-only entries are filtered out",
			watchNamespace:             "dev",
			additionalSourceNamespaces: []string{"flux-system", "", "   "},
			wantDefaultNamespaces:      map[string]ctrlcache.Config{"dev": {}},
			wantByObjectNamespaces:     map[string]ctrlcache.Config{"dev": {}, "flux-system": {}},
		},
		{
			name:                       "an all-blank list behaves the same as no additional namespaces",
			watchNamespace:             "dev",
			additionalSourceNamespaces: []string{"", "  "},
			wantDefaultNamespaces:      map[string]ctrlcache.Config{"dev": {}},
			wantByObjectNamespaces:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCacheOptions(tt.watchNamespace, tt.additionalSourceNamespaces)

			if !reflect.DeepEqual(tt.wantDefaultNamespaces, got.DefaultNamespaces) {
				t.Errorf("DefaultNamespaces = %#v, want %#v", got.DefaultNamespaces, tt.wantDefaultNamespaces)
			}

			if tt.wantByObjectNamespaces == nil {
				if got.ByObject != nil {
					t.Errorf("expected ByObject to be nil, got %#v", got.ByObject)
				}
				return
			}

			if got.ByObject == nil {
				t.Fatalf("expected ByObject to be set for source kinds, got nil")
			}

			// ByObject is keyed by client.Object (an interface holding a pointer).
			// Two separately-constructed &sourcev1.GitRepository{} values are
			// DIFFERENT pointers, so map[key] lookups with a fresh pointer would
			// never match — iterate and type-switch on the key instead.
			seenKinds := map[string]bool{"GitRepository": false, "Bucket": false, "OCIRepository": false}
			for obj, cfg := range got.ByObject {
				var kind string
				switch obj.(type) {
				case *sourcev1.GitRepository:
					kind = "GitRepository"
				case *sourcev1.Bucket:
					kind = "Bucket"
				case *sourcev1.OCIRepository:
					kind = "OCIRepository"
				default:
					t.Fatalf("unexpected ByObject key type %T", obj)
				}
				seenKinds[kind] = true
				if !reflect.DeepEqual(tt.wantByObjectNamespaces, cfg.Namespaces) {
					t.Errorf("ByObject[%s].Namespaces = %#v, want %#v", kind, cfg.Namespaces, tt.wantByObjectNamespaces)
				}
			}
			for kind, seen := range seenKinds {
				if !seen {
					t.Errorf("missing ByObject entry for %s", kind)
				}
			}
		})
	}
}

// distinctMapInstances guards against a subtle regression: buildCacheOptions
// must give each ByObject entry its own Namespaces map instance, not a shared
// one, because controller-runtime's option-defaulting mutates Config values
// in place — sharing one map across GitRepository/Bucket/OCIRepository would
// mean a mutation intended for one GVK silently leaks into the others.
func TestBuildCacheOptionsDistinctMapInstances(t *testing.T) {
	got := buildCacheOptions("dev", []string{"flux-system"})
	seen := map[*ctrlclient.Object]bool{}
	var namespaceMaps []map[string]ctrlcache.Config
	for _, cfg := range got.ByObject {
		namespaceMaps = append(namespaceMaps, cfg.Namespaces)
	}
	if len(namespaceMaps) != 3 {
		t.Fatalf("expected 3 ByObject entries, got %d", len(namespaceMaps))
	}
	for i := 0; i < len(namespaceMaps); i++ {
		for j := i + 1; j < len(namespaceMaps); j++ {
			// reflect on the map header pointer via a mutation probe: mutate one
			// and confirm the others are unaffected.
			namespaceMaps[i]["probe"] = ctrlcache.Config{}
			if _, leaked := namespaceMaps[j]["probe"]; leaked {
				t.Fatalf("Namespaces map instance is shared across ByObject entries — mutation leaked")
			}
			delete(namespaceMaps[i], "probe")
		}
	}
	_ = seen
}
