package config

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/ofmeteoriteh/Meyrin/mdtable"
)

// 规范中两张模型表的锚点 / Anchors of the two model tables in the spec
const (
	subjectAnchor = "受测模型："
	judgeAnchor   = "评审模型："
)

// 凭据与地址变量名的形式 / Shapes of credential and address variable names
var (
	keyEnvName     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_API_KEY$`)
	baseURLEnvName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_BASE_URL$`)
)

// Check 核对模型清单与规范、端点文档、.env.example 一致，返回全部不一致项。
// Check verifies the model list against the spec, the endpoint document and
// .env.example, reporting every mismatch.
func Check(m Models, spec, endpointDoc, envExample string) error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	names := make([]string, len(m.Vendors))
	for i, v := range m.Vendors {
		names[i] = v.Name
	}

	// 受测与评审名单须与规范逐行一致 / Vendor and judge lists must match the spec row by row
	if subjects, err := specColumn(spec, subjectAnchor, []string{"厂商"}); err != nil {
		errs = append(errs, err)
	} else if !slices.Equal(names, subjects[0]) {
		fail("vendors %q, want %q as in the spec", names, subjects[0])
	}
	if judges, err := specColumn(spec, judgeAnchor, []string{"厂商", "调用方式"}); err != nil {
		errs = append(errs, err)
	} else {
		var got [][2]string
		for _, j := range m.Judges {
			got = append(got, [2]string{j.Vendor, j.Access})
		}
		var want [][2]string
		for i := range judges[0] {
			want = append(want, [2]string{judges[0][i], judges[1][i]})
		}
		if !slices.Equal(got, want) {
			fail("judges %q, want %q as in the spec", got, want)
		}
	}

	if endpoints, err := ParseEndpoints(endpointDoc); err != nil {
		errs = append(errs, err)
	} else {
		for _, n := range names {
			if _, ok := endpoints[n]; !ok {
				fail("vendor %q has no endpoint in the endpoint document", n)
			}
		}
		for _, n := range slices.Sorted(maps.Keys(endpoints)) {
			if !slices.Contains(names, n) {
				fail("endpoint vendor %q is not in the model list", n)
			}
		}
		for _, j := range m.Judges {
			if _, ok := endpoints[j.Vendor]; j.Access == AccessAPI && !ok {
				fail("judge %q uses the official API but has no endpoint", j.Vendor)
			}
		}
	}

	keys := map[string]bool{}
	for _, v := range m.Vendors {
		switch {
		case !keyEnvName.MatchString(v.KeyEnv):
			fail("vendor %q: key_env %q must look like NAME_API_KEY", v.Name, v.KeyEnv)
		case keys[v.KeyEnv]:
			fail("vendor %q: key_env %q is used twice", v.Name, v.KeyEnv)
		}
		keys[v.KeyEnv] = true
		if _, err := v.ProxyURL(); err != nil {
			errs = append(errs, err)
		}
	}

	// 评审凭据独立于受测凭据；订阅方式另需地址变量 / Judge keys are separate; subscriptions also need an address variable
	for _, j := range m.Judges {
		switch {
		case !keyEnvName.MatchString(j.KeyEnv):
			fail("judge %q: key_env %q must look like NAME_API_KEY", j.Vendor, j.KeyEnv)
		case keys[j.KeyEnv]:
			fail("judge %q: key_env %q is used twice; judges need their own keys", j.Vendor, j.KeyEnv)
		}
		keys[j.KeyEnv] = true
		switch j.Access {
		case AccessSubscription:
			if !baseURLEnvName.MatchString(j.BaseURLEnv) {
				fail("judge %q: base_url_env %q must look like NAME_BASE_URL", j.Vendor, j.BaseURLEnv)
			}
			keys[j.BaseURLEnv] = true
		case AccessAPI:
			if j.BaseURLEnv != "" {
				fail("judge %q: base_url_env is only for subscriptions", j.Vendor)
			}
		default:
			fail("judge %q: access %q, want %q or %q", j.Vendor, j.Access, AccessAPI, AccessSubscription)
		}
	}

	// .env.example 须恰好列出全部凭据与地址变量 / .env.example must list exactly the credential and address variables
	if example, err := ParseEnv(envExample); err != nil {
		fail(".env.example: %w", err)
	} else {
		for _, k := range slices.Sorted(maps.Keys(keys)) {
			if _, ok := example[k]; !ok {
				fail(".env.example lacks %s", k)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(example)) {
			if !keys[k] {
				fail(".env.example has %s, which no vendor or judge uses", k)
			}
		}
	}
	return errors.Join(errs...)
}

// specColumn 读取规范中锚点下的表格，按列返回单元格 / specColumn reads the table under anchor column by column
func specColumn(spec, anchor string, header []string) ([][]string, error) {
	tbl, err := mdtable.After(spec, anchor)
	if err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	if err := tbl.Check(header...); err != nil {
		return nil, fmt.Errorf("spec %s: %w", anchor, err)
	}
	cols := make([][]string, len(header))
	for i, h := range header {
		cols[i], _ = tbl.Column(h)
	}
	return cols, nil
}
