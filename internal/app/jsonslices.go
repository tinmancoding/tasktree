package app

import "encoding/json"

// JSON documents never carry null for a list: an empty list is []. The zero
// value of each report type (returned on early errors) must obey the same rule.

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// MarshalJSON encodes nil lists as [].
func (r SyncReport) MarshalJSON() ([]byte, error) {
	type plain SyncReport
	r.Steps, r.Dirty, r.Diverged, r.Skipped = nonNil(r.Steps), nonNil(r.Dirty), nonNil(r.Diverged), nonNil(r.Skipped)
	r.UnpushedGitlinks, r.CheckoutsBehind = nonNil(r.UnpushedGitlinks), nonNil(r.CheckoutsBehind)
	r.ClosedTaskCheckouts, r.Failed = nonNil(r.ClosedTaskCheckouts), nonNil(r.Failed)
	return json.Marshal(plain(r))
}

// MarshalJSON encodes nil lists as [].
func (r ResolveResult) MarshalJSON() ([]byte, error) {
	type plain ResolveResult
	r.LocalCommits, r.RemoteCommits = nonNil(r.LocalCommits), nonNil(r.RemoteCommits)
	r.Paths, r.AutoMerged = nonNil(r.Paths), nonNil(r.AutoMerged)
	return json.Marshal(plain(r))
}

// MarshalJSON encodes nil lists as [].
func (r DoctorResult) MarshalJSON() ([]byte, error) {
	type plain DoctorResult
	r.Findings = nonNil(r.Findings)
	return json.Marshal(plain(r))
}

// MarshalJSON encodes nil lists as [].
func (i TasktreeInfo) MarshalJSON() ([]byte, error) {
	type plain TasktreeInfo
	i.Repos = nonNil(i.Repos)
	return json.Marshal(plain(i))
}

// MarshalJSON encodes nil lists as [].
func (i DefinitionInfo) MarshalJSON() ([]byte, error) {
	type plain DefinitionInfo
	i.Repos = nonNil(i.Repos)
	return json.Marshal(plain(i))
}

// MarshalJSON encodes nil lists as [].
func (d TemplateDetail) MarshalJSON() ([]byte, error) {
	type plain TemplateDetail
	d.Parameters = nonNil(d.Parameters)
	return json.Marshal(plain(d))
}
