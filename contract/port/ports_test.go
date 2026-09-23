package port

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPortIsExactlyTheRequiredPublicContractSurface(t *testing.T) {
	bytes, err := os.ReadFile("../upstream/dolgorae-gul-consumer-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		RequiredMethods []string `json:"required_methods"`
	}
	if err := json.Unmarshal(bytes, &contract); err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, len(contract.RequiredMethods))
	for _, name := range contract.RequiredMethods {
		parts := strings.Split(name, ".")
		want = append(want, parts[len(parts)-1])
	}
	slices.Sort(want)
	port := reflect.TypeOf((*PublicContractPort)(nil)).Elem()
	actual := make([]string, port.NumMethod())
	for index := range actual {
		actual[index] = port.Method(index).Name
	}
	slices.Sort(actual)
	if !slices.Equal(actual, want) {
		t.Fatalf("provider port surface = %v; required consumer methods = %v", actual, want)
	}
}
