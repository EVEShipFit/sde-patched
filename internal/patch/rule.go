package patch

import (
	"fmt"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"

	"gopkg.in/yaml.v3"
)

// A rule is written with the operation as the key that carries its input:
//
//	rules:
//	  - {from: armorHP}
//	  - {div: armorDamageEffectiveResonance}
//
// The operations are listed in the order dogma applies them. "from" is
// preAssign and "assign" is postAssign.

var operations = []struct {
	word string
	op   eve.ModifierOperation
}{
	{"from", eve.ModifierOperationPreAssign},
	{"mulFirst", eve.ModifierOperationPreMul},
	{"divFirst", eve.ModifierOperationPreDiv},
	{"add", eve.ModifierOperationModAdd},
	{"sub", eve.ModifierOperationModSub},
	{"mul", eve.ModifierOperationPostMul},
	{"div", eve.ModifierOperationPostDiv},
	{"percent", eve.ModifierOperationPostPercent},
	{"assign", eve.ModifierOperationPostAssign},
}

var operationByWord = func() map[string]eve.ModifierOperation {
	found := map[string]eve.ModifierOperation{}
	for _, entry := range operations {
		found[entry.word] = entry.op
	}
	return found
}()

// Operations are the words a rule may carry its input under, in the order
// dogma applies them.
func Operations() []string {
	words := make([]string, 0, len(operations))
	for _, entry := range operations {
		words = append(words, entry.word)
	}
	return words
}

// OperationWord is how a rule spells one of dogma's operations, for reading
// back what is already in the SDE.
func OperationWord(op int32) string {
	for _, entry := range operations {
		if int32(entry.op) == op {
			return entry.word
		}
	}
	return ""
}

func operationOf(word string) (eve.ModifierOperation, error) {
	op, ok := operationByWord[word]
	if !ok {
		return 0, fmt.Errorf("unknown operation %q, want one of %s", word, strings.Join(Operations(), ", "))
	}
	return op, nil
}

func (r *Rule) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a rule is a mapping, not %s", node.Line, kindName(node.Kind))
	}

	*r = Rule{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]

		if _, isOp := operationByWord[key.Value]; isOp {
			if r.Op != "" {
				return fmt.Errorf("line %d: a rule does one thing, but this one says both %q and %q", key.Line, r.Op, key.Value)
			}
			r.Op, r.By = key.Value, value.Value
			continue
		}

		switch key.Value {
		case "domain":
			r.Domain = value.Value
		case "func":
			r.Func = value.Value
		case "group":
			r.Group = value.Value
		case "skill":
			r.Skill = value.Value
		default:
			return fmt.Errorf("line %d: unknown key %q, want one of %s", key.Line, key.Value, strings.Join(Operations(), ", "))
		}
	}

	if r.Op == "" {
		return fmt.Errorf("line %d: a rule needs one of %s: what it writes with", node.Line, strings.Join(Operations(), ", "))
	}
	return nil
}

func (r Rule) MarshalYAML() (any, error) {
	if _, err := operationOf(r.Op); err != nil {
		return nil, err
	}

	node := &yaml.Node{Kind: yaml.MappingNode}
	put := func(key, value string) {
		if value == "" {
			return
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value})
	}

	put(r.Op, r.By)
	put("domain", r.Domain)
	put("func", r.Func)
	put("group", r.Group)
	put("skill", r.Skill)
	return node, nil
}

// domain and function are what nearly every rule wants, so a rule only says
// them when it wants something else.
func (r Rule) domain() string {
	if r.Domain == "" {
		return "itemID"
	}
	return r.Domain
}

func (r Rule) function() string {
	if r.Func == "" {
		return "itemModifier"
	}
	return r.Func
}
