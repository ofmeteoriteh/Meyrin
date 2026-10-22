// Package stats 实现规范「一致性检验」与「汇总」：结果项的 Krippendorff's α、风格标签的
// positive specific agreement、按回答实例的 bootstrap 置信区间与区间判定、自评偏差，
// 以及记录表所需的逐格汇总与人工复核清单。
//
// Package stats implements the spec's reliability checks and aggregation: Krippendorff's
// α for outcomes, positive specific agreement for style tags, bootstrap confidence
// intervals over answers with the interval rules, self-preference bias, and the per-cell
// summary and human review list behind the record table.
package stats

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Alpha 计算名义尺度的 Krippendorff's α。每个单元是各评审给出的有效取值（缺失已去除），
// 少于 2 个取值的单元不可配对、不参与计算；期望不一致为 0（全部取值相同）时无定义，ok 为 false。
//
// Alpha computes nominal Krippendorff's α. Each unit holds the judges' valid values with
// missing ones removed; units with fewer than two values cannot be paired and are skipped.
// α is undefined, with ok false, when expected disagreement is zero.
func Alpha(units [][]string) (alpha float64, ok bool) {
	index := map[string]int{}
	var values []string
	for _, u := range units {
		for _, v := range u {
			if _, seen := index[v]; !seen {
				index[v] = 0
				values = append(values, v)
			}
		}
	}
	sort.Strings(values)
	for i, v := range values {
		index[v] = i
	}

	// 一致矩阵按排序后的取值索引累加，结果与遍历顺序无关 / The coincidence matrix is indexed by sorted values, so sums are order-independent
	coincidence := make([][]float64, len(values))
	for i := range coincidence {
		coincidence[i] = make([]float64, len(values))
	}
	for _, u := range units {
		m := len(u)
		if m < 2 {
			continue
		}
		for i := range u {
			for j := range u {
				if i != j {
					coincidence[index[u[i]]][index[u[j]]] += 1 / float64(m-1)
				}
			}
		}
	}
	totals := make([]float64, len(values))
	n, observed := 0.0, 0.0
	for c := range values {
		for k := range values {
			totals[c] += coincidence[c][k]
			n += coincidence[c][k]
			if c != k {
				observed += coincidence[c][k]
			}
		}
	}
	expected := 0.0
	for c := range values {
		for k := range values {
			if c != k {
				expected += totals[c] * totals[k]
			}
		}
	}
	if expected == 0 {
		return 0, false
	}
	return 1 - (n-1)*observed/expected, true
}

// TagUnit 是一条回答上某个标签的有效票：K 为判 present 或 absent 的评审数，Pos 为判 present 的评审数。
// TagUnit holds one answer's valid votes on a tag: K judges voted present or absent, Pos of them present.
type TagUnit struct {
	Pos, K int
}

// PSA 计算 positive specific agreement：Σ Pos×(Pos−1) / Σ Pos×(K−1)；分母为 0 时无定义。
// PSA computes positive specific agreement, undefined when the denominator is zero.
func PSA(units []TagUnit) (psa float64, ok bool) {
	num, den := 0, 0
	for _, u := range units {
		num += u.Pos * (u.Pos - 1)
		den += u.Pos * (u.K - 1)
	}
	if den == 0 {
		return 0, false
	}
	return float64(num) / float64(den), true
}

// Interval 是 95% bootstrap 置信区间；可用的重采样少于一半时无定义。
// Interval is a 95% bootstrap confidence interval; it is undefined when fewer than half the resamples are usable.
type Interval struct {
	Low  *float64 `json:"low"`
	High *float64 `json:"high"`
}

// bootstrap 以回答为单位有放回重采样 b 次，取统计量的 2.5% 与 97.5% 分位数；
// stat 对某次重采样无定义时该次舍弃。
//
// bootstrap resamples answers with replacement b times and takes the 2.5% and 97.5%
// quantiles of the statistic, discarding resamples where it is undefined.
func bootstrap(rng *rand.Rand, b int, sizes []int, stat func(samples [][]int) (float64, bool)) Interval {
	values := make([]float64, 0, b)
	for range b {
		samples := make([][]int, len(sizes))
		for g, n := range sizes {
			samples[g] = make([]int, n)
			for i := range n {
				samples[g][i] = rng.IntN(n)
			}
		}
		if v, ok := stat(samples); ok && !math.IsNaN(v) {
			values = append(values, v)
		}
	}
	if len(values) == 0 || len(values) < b/2 {
		return Interval{}
	}
	sort.Float64s(values)
	low := quantile(values, 0.025)
	high := quantile(values, 0.975)
	return Interval{Low: &low, High: &high}
}

// quantile 按线性插值取已排序数据的分位数 / quantile interpolates a quantile of sorted data
func quantile(sorted []float64, p float64) float64 {
	pos := p * float64(len(sorted)-1)
	i := int(pos)
	if i+1 >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[i] + (pos-float64(i))*(sorted[i+1]-sorted[i])
}

// 区间判定，阈值与措辞见规范「一致性检验」 / Interval decisions; thresholds and wording follow the spec
const (
	DecisionUsable       = "可用"
	DecisionTentative    = "暂定"
	DecisionRevise       = "修订"
	DecisionInsufficient = "证据不足"

	alphaUsable    = 0.800
	alphaTentative = 0.667
	psaThreshold   = 0.5
)

// decideAlpha 按 α 置信区间判定：下限 ≥ 0.800 可用，≥ 0.667 暂定，上限 < 0.667 修订，其余证据不足。
// decideAlpha applies the α interval rules.
func decideAlpha(ci Interval) string {
	switch {
	case ci.Low == nil:
		return DecisionInsufficient
	case *ci.Low >= alphaUsable:
		return DecisionUsable
	case *ci.Low >= alphaTentative:
		return DecisionTentative
	case *ci.High < alphaTentative:
		return DecisionRevise
	}
	return DecisionInsufficient
}

// decidePSA 按 PSA 置信区间判定：下限 ≥ 0.5 可用，上限 < 0.5 修订，其余证据不足。
// decidePSA applies the PSA interval rules.
func decidePSA(ci Interval) string {
	switch {
	case ci.Low == nil:
		return DecisionInsufficient
	case *ci.Low >= psaThreshold:
		return DecisionUsable
	case *ci.High < psaThreshold:
		return DecisionRevise
	}
	return DecisionInsufficient
}
