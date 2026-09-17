package sde

import (
	"encoding/json"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
)

func TestDbuffCollectionUnmarshal(t *testing.T) {
	raw := `{
		"_key": 11,
		"aggregateMode": "Minimum",
		"displayName": {"en": "Shield Repair Modules"},
		"itemModifiers": [{"dogmaAttributeID": 263}],
		"locationModifiers": [{"dogmaAttributeID": 68}],
		"locationGroupModifiers": [{"dogmaAttributeID": 54, "groupID": 208}],
		"locationRequiredSkillModifiers": [{"dogmaAttributeID": 73, "skillID": 3416}],
		"operationName": "PostAssignment",
		"showOutputValueInUI": "ShowInverted"
	}`

	var buff DbuffCollection
	if err := json.Unmarshal([]byte(raw), &buff); err != nil {
		t.Fatal(err)
	}

	if buff.Key != 11 || buff.DisplayName.En != "Shield Repair Modules" {
		t.Errorf("buff = %d, %q", buff.Key, buff.DisplayName.En)
	}
	if buff.AggregateMode != eve.DbuffAggregateModeMinimum {
		t.Errorf("aggregate mode = %v", buff.AggregateMode)
	}
	// The SDE spells it "PostAssignment" here, "PostAssign" on an effect.
	if buff.Operation != eve.ModifierOperationPostAssign {
		t.Errorf("operation = %v", buff.Operation)
	}
	if buff.Display != eve.DbuffDisplayInverted {
		t.Errorf("display = %v", buff.Display)
	}

	// One modifier per entry of the four lists, in that order.
	want := []DbuffModifier{
		{Func: eve.ModifierFuncItemModifier, ModifiedAttributeID: 263},
		{Func: eve.ModifierFuncLocationModifier, ModifiedAttributeID: 68},
		{Func: eve.ModifierFuncLocationGroupModifier, ModifiedAttributeID: 54, GroupID: 208},
		{Func: eve.ModifierFuncLocationRequiredSkillModifier, ModifiedAttributeID: 73, SkillTypeID: 3416},
	}
	if len(buff.Modifiers) != len(want) {
		t.Fatalf("modifiers = %d, want %d", len(buff.Modifiers), len(want))
	}
	for i, modifier := range want {
		if buff.Modifiers[i] != modifier {
			t.Errorf("modifier %d = %+v, want %+v", i, buff.Modifiers[i], modifier)
		}
	}
}

func TestDbuffCollectionUnmarshalUnknownOperation(t *testing.T) {
	var buff DbuffCollection
	if err := json.Unmarshal([]byte(`{"_key": 1, "aggregateMode": "Maximum", "operationName": "Nope", "showOutputValueInUI": "ShowNormal"}`), &buff); err == nil {
		t.Error("an unknown operation should not load")
	}
}

func TestDbuffCollectionUnmarshalUnknownDisplay(t *testing.T) {
	var buff DbuffCollection
	if err := json.Unmarshal([]byte(`{"_key": 1, "aggregateMode": "Maximum", "operationName": "ModAdd", "showOutputValueInUI": "Nope"}`), &buff); err == nil {
		t.Error("an unknown display should not load")
	}
}
