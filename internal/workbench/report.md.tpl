# Experiment {{shorten .ID}}

**Date:** {{fmtDate .CreatedAt}} | **Status:** {{.Status}} | **ID:** {{shorten .ID}}

---

## Target

- Round {{.Round.Round}} of activation {{shorten .Round.ActivationID}}
- Model: {{.Activation.Model}}{{if .Activation.ReasoningEffort}} | Effort: {{.Activation.ReasoningEffort}}{{end}}{{if .Activation.Verbosity}} | Verbosity: {{.Activation.Verbosity}}{{end}}

### Actual Response
{{if .Round.ModelOutput}}
> {{truncate .Round.ModelOutput 500}}
{{end}}{{if .ToolCalls}}Tool calls: {{tcNames .ToolCalls}}
{{end}}
{{- if .DesiredOutcome}}
---

## Desired Outcome

{{.DesiredOutcome}}
{{end}}
{{- if .Analysis}}
---

## Analysis

{{.Analysis}}
{{end}}
---

## Variants
{{- if hasRuns .Variants}}

| # | Rate | Variant | Runs | Hit | Miss | Link |
|---|------|---------|------|-----|------|------|
{{- range $i, $v := .Variants}}{{if $v.Runs}}
| v{{$i}} | {{printf "%.0f" (rate $v.DesiredCount $v.RunCount)}}% | {{$v.Name}} | {{$v.RunCount}} | {{$v.DesiredCount}} | {{sub $v.RunCount $v.DesiredCount}} | [details](#{{anchor $i $v.Name}}) |
{{- end}}{{end}}
{{end}}
{{range $i, $v := .Variants}}
{{- if $v.Runs}}
### v{{$i}} — {{$v.Name}} — {{$v.DesiredCount}} hit, {{sub $v.RunCount $v.DesiredCount}} miss ({{printf "%.0f" (rate $v.DesiredCount $v.RunCount)}}%)
{{- else}}
### v{{$i}} — {{$v.Name}} — pending
{{- end}}
{{- if $v.Hypothesis}}

**Why:** {{$v.Hypothesis}}
{{- end}}
{{- if $v.Patches}}

```diff
{{$v.Patches}}```
{{- end}}
{{- if $v.Runs}}

| # | Desired | Rationale | Model Output | Tools | Tokens (in/out) |
|---|---------|-----------|--------------|-------|------------------|
{{- range $j, $r := $v.Runs}}
| {{add $j 1}} | {{desiredStr $r.IsDesired}} | {{defaultStr $r.Rationale "-"}} | {{defaultStr (truncate $r.ModelOutput 120) "(none)"}} | {{toolCallNames $r.ToolCalls}} | {{$r.InputTokens}}/{{$r.OutputTokens}} |
{{- end}}
{{end}}
{{end}}
