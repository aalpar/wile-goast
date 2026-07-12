// Package dispatch_reflect is a scratch reproduction of the reflective-registry
// idiom (encoding/json, database/sql, protobuf, apimachinery's runtime.Scheme):
// a concrete type reaches an interface ONLY through reflect.New(...).Interface(),
// never through an ssa.MakeInterface VTA can see.
package dispatch_reflect

import "reflect"

type Greeter interface{ Greet() string }

type Alpha struct{}

func (Alpha) Greet() string { return "alpha" }

type Beta struct{}

func (Beta) Greet() string { return "beta" }

var registry = map[string]reflect.Type{
	"alpha": reflect.TypeOf(Alpha{}),
	"beta":  reflect.TypeOf(Beta{}),
}

// Make returns a Greeter. The "alpha" branch is the ONLY explicit
// MakeInterface conversion in this package, so it is the ONLY type VTA ever
// sees flow into Greeter. The "beta" (and default) branch builds a Greeter
// via reflection -- reflect.New(t).Elem().Interface().(Greeter) -- which
// never appears in a MakeInterface, so VTA cannot see Beta flow in at all.
// Both branches return through the SAME function result, so VTA merges them:
// its flow set for Make's result is {Alpha} only.
func Make(name string) Greeter {
	if name == "alpha" {
		return Alpha{}
	}
	t := registry[name]
	v := reflect.New(t).Elem().Interface()
	g, _ := v.(Greeter)
	return g
}

// CallSite: ground truth is (Beta).Greet -- the tool reports `must`
// candidate=Alpha, WRONG, because Alpha is the only type VTA ever saw flow
// into Greeter anywhere in this package.
func CallSite() string {
	g := Make("beta")
	return g.Greet()
}
