package protomcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ActionDef struct {
	Name        string
	Description string
	Args        []ArgDef
	HandlerFn   func(ToolContext, map[string]interface{}) ToolResult
	Requires    []string
	EnumFields  map[string][]string
}

type GroupDef struct {
	Name        string
	Description string
	Actions     []ActionDef
	Strategy    string // "union" or "separate"
}

type GroupOption func(*GroupDef)
type ActionOption func(*ActionDef)

var groupRegistry []GroupDef

func ToolGroup(name string, opts ...GroupOption) {
	gd := GroupDef{
		Name:     name,
		Strategy: "union",
	}
	for _, opt := range opts {
		opt(&gd)
	}
	groupRegistry = append(groupRegistry, gd)
}

func GroupDescription(desc string) GroupOption {
	return func(gd *GroupDef) { gd.Description = desc }
}

func GroupStrategy(strategy string) GroupOption {
	return func(gd *GroupDef) { gd.Strategy = strategy }
}

func Action(name string, opts ...ActionOption) GroupOption {
	return func(gd *GroupDef) {
		ad := ActionDef{
			Name:       name,
			EnumFields: map[string][]string{},
		}
		for _, opt := range opts {
			opt(&ad)
		}
		gd.Actions = append(gd.Actions, ad)
	}
}

func ActionDescription(desc string) ActionOption {
	return func(ad *ActionDef) { ad.Description = desc }
}

func ActionArgs(args ...ArgDef) ActionOption {
	return func(ad *ActionDef) { ad.Args = args }
}

func ActionHandler(fn func(ToolContext, map[string]interface{}) ToolResult) ActionOption {
	return func(ad *ActionDef) { ad.HandlerFn = fn }
}

func ActionRequires(reqs ...string) ActionOption {
	return func(ad *ActionDef) { ad.Requires = reqs }
}

func ActionEnumField(field string, values []string) ActionOption {
	return func(ad *ActionDef) { ad.EnumFields[field] = values }
}

func GroupsToToolDefs() []ToolDef {
	var defs []ToolDef
	for _, g := range groupRegistry {
		if g.Strategy == "separate" {
			defs = append(defs, groupToSeparateDefs(g)...)
		} else {
			defs = append(defs, groupToUnionDef(g))
		}
	}
	return defs
}

func groupToUnionDef(g GroupDef) ToolDef {
	actionNames := make([]interface{}, len(g.Actions))
	for i, a := range g.Actions {
		actionNames[i] = a.Name
	}

	oneOf := make([]interface{}, len(g.Actions))
	for i, a := range g.Actions {
		props := map[string]interface{}{
			"action": map[string]interface{}{"const": a.Name},
		}
		required := []interface{}{"action"}
		for _, arg := range a.Args {
			props[arg.Name] = argToSchema(arg)
			required = append(required, arg.Name)
		}
		oneOf[i] = map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": actionNames,
			},
		},
		"required": []interface{}{"action"},
		"oneOf":    oneOf,
	}

	desc := g.Description
	names := make([]string, len(g.Actions))
	for i, a := range g.Actions {
		names[i] = a.Name
	}
	actionList := strings.Join(names, ", ")
	if desc != "" {
		desc += " Actions: " + actionList
	} else {
		desc = "Actions: " + actionList
	}

	gCopy := g
	return ToolDef{
		Name:        g.Name,
		Desc:        desc,
		InputSchema: schema,
		HandlerFn: func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return DispatchGroupAction(gCopy, ctx, args)
		},
	}
}

func groupToSeparateDefs(g GroupDef) []ToolDef {
	var defs []ToolDef
	for _, a := range g.Actions {
		aCopy := a
		props := map[string]interface{}{}
		required := []string{}
		for _, arg := range a.Args {
			props[arg.Name] = argToSchema(arg)
			required = append(required, arg.Name)
		}
		schema := map[string]interface{}{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		desc := a.Description
		if desc == "" {
			desc = g.Name + " " + a.Name
		}
		defs = append(defs, ToolDef{
			Name:        fmt.Sprintf("%s.%s", g.Name, a.Name),
			Desc:        desc,
			InputSchema: schema,
			HandlerFn: func(ctx ToolContext, args map[string]interface{}) ToolResult {
				if aCopy.HandlerFn != nil {
					return aCopy.HandlerFn(ctx, args)
				}
				return Result("")
			},
		})
	}
	return defs
}

func DispatchGroupAction(g GroupDef, ctx ToolContext, args map[string]interface{}) ToolResult {
	actionName, ok := args["action"].(string)
	if !ok {
		names := make([]string, len(g.Actions))
		for i, a := range g.Actions {
			names[i] = a.Name
		}
		return ErrorResult(
			fmt.Sprintf("Missing 'action' field. Available actions: %s", strings.Join(names, ", ")),
			"MISSING_ACTION", "", false,
		)
	}

	for _, a := range g.Actions {
		if a.Name == actionName {
			if a.HandlerFn == nil {
				return Result("")
			}
			// Remove action from args before dispatch
			argsCopy := make(map[string]interface{})
			for k, v := range args {
				if k != "action" {
					argsCopy[k] = v
				}
			}
			return a.HandlerFn(ctx, argsCopy)
		}
	}

	names := make([]string, len(g.Actions))
	for i, a := range g.Actions {
		names[i] = a.Name
	}
	suggestion := fuzzyMatch(actionName, names)
	msg := fmt.Sprintf("Unknown action '%s'.", actionName)
	if suggestion != "" {
		msg += fmt.Sprintf(" Did you mean '%s'?", suggestion)
	}
	msg += fmt.Sprintf(" Available actions: %s", strings.Join(names, ", "))
	return ErrorResult(msg, "UNKNOWN_ACTION", suggestion, false)
}

// fuzzyMatch returns the closest match using simple edit distance.
func fuzzyMatch(input string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	best := ""
	bestDist := len(input) + 10
	for _, c := range candidates {
		d := levenshtein(strings.ToLower(input), strings.ToLower(c))
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	// Only suggest if the distance is less than half the input length
	threshold := len(input)/2 + 1
	if bestDist <= threshold {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	matrix := make([][]int, la+1)
	for i := range matrix {
		matrix[i] = make([]int, lb+1)
		matrix[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		matrix[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min3(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}
	return matrix[la][lb]
}

func min3(a, b, c int) int {
	arr := []int{a, b, c}
	sort.Ints(arr)
	return arr[0]
}

func GetRegisteredGroups() []GroupDef { return append([]GroupDef{}, groupRegistry...) }

func ClearGroupRegistry() { groupRegistry = nil }

func (gd GroupDef) InputSchemaJSON() string {
	// Build union schema for the group
	td := groupToUnionDef(gd)
	b, _ := json.Marshal(td.InputSchema)
	return string(b)
}
