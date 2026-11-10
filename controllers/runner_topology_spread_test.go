package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const topologySpreadJSON = `[{
  "maxSkew": 10,
  "minDomains": 100,
  "topologyKey": "kubernetes.io/hostname",
  "whenUnsatisfiable": "DoNotSchedule",
  "nodeAffinityPolicy": "Honor",
  "nodeTaintsPolicy": "Honor",
  "labelSelector": {"matchLabels": {"app.kubernetes.io/created-by": "tofu-controller"}}
}]`

const topologySpreadYAML = `
- maxSkew: 10
  minDomains: 100
  topologyKey: kubernetes.io/hostname
  whenUnsatisfiable: DoNotSchedule
  nodeAffinityPolicy: Honor
  nodeTaintsPolicy: Honor
  labelSelector:
    matchLabels:
      app.kubernetes.io/created-by: tofu-controller
`

func TestParseRunnerTopologySpreadConstraints(t *testing.T) {
	expected := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:            10,
			MinDomains:         ptr.To(int32(100)),
			TopologyKey:        "kubernetes.io/hostname",
			WhenUnsatisfiable:  corev1.DoNotSchedule,
			NodeAffinityPolicy: ptr.To(corev1.NodeInclusionPolicyHonor),
			NodeTaintsPolicy:   ptr.To(corev1.NodeInclusionPolicyHonor),
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/created-by": "tofu-controller"},
			},
		},
	}

	tests := []struct {
		name    string
		input   string
		want    []corev1.TopologySpreadConstraint
		wantErr string
	}{
		{name: "empty", input: ""},
		{name: "whitespace only", input: "  \n\t "},
		{name: "valid JSON", input: topologySpreadJSON, want: expected},
		{name: "valid YAML", input: topologySpreadYAML, want: expected},
		{
			name:  "ScheduleAnyway without minDomains",
			input: `[{"maxSkew":1,"topologyKey":"zone","whenUnsatisfiable":"ScheduleAnyway"}]`,
			want: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "zone", WhenUnsatisfiable: corev1.ScheduleAnyway},
			},
		},
		{name: "invalid syntax", input: `[{"maxSkew":`, wantErr: "parse"},
		{
			name:    "not a list",
			input:   `{"maxSkew":1}`,
			wantErr: "parse",
		},
		{
			name:    "unknown field",
			input:   `[{"maxSkew":1,"topologyKey":"zone","whenUnsatisfiable":"DoNotSchedule","maxSkewz":2}]`,
			wantErr: "maxSkewz",
		},
		{
			name:    "missing topologyKey",
			input:   `[{"maxSkew":1,"whenUnsatisfiable":"DoNotSchedule"}]`,
			wantErr: "constraint 0: topologyKey",
		},
		{
			name: "maxSkew zero reports the index",
			input: `[{"maxSkew":1,"topologyKey":"zone","whenUnsatisfiable":"DoNotSchedule"},
			         {"maxSkew":0,"topologyKey":"zone","whenUnsatisfiable":"DoNotSchedule"}]`,
			wantErr: "constraint 1: maxSkew",
		},
		{
			name:    "bad whenUnsatisfiable",
			input:   `[{"maxSkew":1,"topologyKey":"zone","whenUnsatisfiable":"Never"}]`,
			wantErr: "whenUnsatisfiable",
		},
		{
			name:    "missing whenUnsatisfiable",
			input:   `[{"maxSkew":1,"topologyKey":"zone"}]`,
			wantErr: "whenUnsatisfiable",
		},
		{
			name:    "minDomains with ScheduleAnyway",
			input:   `[{"maxSkew":1,"minDomains":3,"topologyKey":"zone","whenUnsatisfiable":"ScheduleAnyway"}]`,
			wantErr: "minDomains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRunnerTopologySpreadConstraints(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRunnerTopologySpreadConstraints_JSONAndYAMLAgree(t *testing.T) {
	fromJSON, err := ParseRunnerTopologySpreadConstraints(topologySpreadJSON)
	require.NoError(t, err)
	fromYAML, err := ParseRunnerTopologySpreadConstraints(topologySpreadYAML)
	require.NoError(t, err)

	assert.NotEmpty(t, fromJSON)
	assert.Equal(t, fromJSON, fromYAML)
}
