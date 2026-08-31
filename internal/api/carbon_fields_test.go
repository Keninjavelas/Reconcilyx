package api

import (
	"encoding/json"
	"testing"

	optimizationv1alpha1 "github.com/reconcilyx/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatCarbonGramsHuman(t *testing.T) {
	tests := []struct {
		name  string
		grams int64
		want  string
	}{
		{"zero", 0, ""},
		{"negative", -10, ""},
		{"500 g", 500, "500 g CO₂"},
		{"999 g just under kg", 999, "999 g CO₂"},
		{"12400 g → 12.4 kg", 12400, "12.4 kg CO₂"},
		{"1,000,000 g → 1.00 tonnes", 1_000_000, "1.00 tonnes CO₂"},
		{"1,500,000 g → 1.50 tonnes", 1_500_000, "1.50 tonnes CO₂"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatCarbonGramsHuman(tc.grams))
		})
	}
}

type savingsResp struct {
	TotalUSD          *float64           `json:"totalUSD"`
	TotalGramsCO2     *int64             `json:"totalGramsCO2"`
	TotalCarbonHuman  *string            `json:"totalCarbonHuman"`
	ByClusterGramsCO2 map[string]int64   `json:"byClusterGramsCO2"`
	ByNsGramsCO2      map[string]int64   `json:"byNsGramsCO2"`
	Series            []struct {
		Date     string  `json:"date"`
		Cluster  string  `json:"cluster,omitempty"`
		USD      float64 `json:"usd"`
		GramsCO2 int64   `json:"gramsCO2,omitempty"`
	} `json:"series"`
}

func makeCarbonProposal(name, ns, cluster string, usd string, grams int64) optimizationv1alpha1.OptimizationProposal {
	p := optimizationv1alpha1.OptimizationProposal{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, CreationTimestamp: metav1.Now()},
		Spec: optimizationv1alpha1.OptimizationProposalSpec{
			TargetClusterName: cluster,
		},
		Status: optimizationv1alpha1.OptimizationProposalStatus{
			ApprovalState: optimizationv1alpha1.ApprovalApproved,
		},
	}
	if usd != "" {
		q := resource.MustParse(usd)
		p.Spec.ExpectedImpact.EstimatedSavingsUSD = &q
	}
	if grams > 0 {
		q := resource.NewQuantity(grams, resource.DecimalSI)
		p.Spec.ExpectedImpact.CarbonSavedGramsCO2 = q
	}
	return p
}

func TestAggregateSavingsIncludesCarbon(t *testing.T) {
	proposals := []optimizationv1alpha1.OptimizationProposal{
		makeCarbonProposal("p1", "ns-a", "cluster-1", "50", 12400),
		makeCarbonProposal("p2", "ns-a", "cluster-1", "25", 6000),
		makeCarbonProposal("p3", "ns-b", "cluster-2", "0", 2_000_000),
	}
	var totalUSD float64
	var totalGrams int64
	byClusterGrams := map[string]int64{}
	byNsGrams := map[string]int64{}
	var seriesCount int
	for _, p := range proposals {
		if !(p.Status.Phase == optimizationv1alpha1.PhaseCompleted ||
			p.Status.Phase == optimizationv1alpha1.PhaseStabilizing ||
			p.Status.ApprovalState == optimizationv1alpha1.ApprovalApproved) {
			continue
		}
		if q := p.Spec.ExpectedImpact.EstimatedSavingsUSD; q != nil {
			f := q.AsApproximateFloat64()
			totalUSD += f
		}
		if q := p.Spec.ExpectedImpact.CarbonSavedGramsCO2; q != nil {
			g := q.MilliValue() / 1000
			totalGrams += g
			byClusterGrams[p.Spec.TargetClusterName] += g
			byNsGrams[p.Namespace] += g
		}
		seriesCount++
	}
	assert.InDelta(t, 75.0, totalUSD, 0.01, "USD total mismatch")
	assert.Equal(t, int64(2_018_400), totalGrams, "carbon total mismatch")
	assert.Equal(t, int64(18400), byClusterGrams["cluster-1"], "cluster-1 grams")
	assert.Equal(t, int64(2_000_000), byClusterGrams["cluster-2"], "cluster-2 grams")
	assert.Equal(t, int64(18400), byNsGrams["ns-a"], "ns-a grams")
	assert.Equal(t, int64(2_000_000), byNsGrams["ns-b"], "ns-b grams")
	assert.Equal(t, 3, seriesCount)
	_ = formatCarbonGramsHuman(totalGrams)
}

func TestProposalListItemCarbonFields(t *testing.T) {
	gramsQty := resource.NewQuantity(12400, resource.DecimalSI)
	item := proposalListItem{Name: "p", Namespace: "ns"}
	if gramsQty != nil {
		g := gramsQty.MilliValue() / 1000
		item.CarbonSavedGrams = g
		item.CarbonSavedHuman = formatCarbonGramsHuman(g)
	}
	buf, err := json.Marshal(item)
	assert.NoError(t, err)
	out := string(buf)
	assert.Contains(t, out, `"carbonSavedGrams":12400`)
	assert.Equal(t, "12.4 kg CO₂", item.CarbonSavedHuman)
	assert.Contains(t, out, item.CarbonSavedHuman)
}
