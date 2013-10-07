package rhttp

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"net/http"
)

type ResponseWriterMock int
func (*ResponseWriterMock) Header() http.Header { return make(http.Header); }
func (*ResponseWriterMock) Write(b []byte) (int, error) { return len(b), nil; }
func (r *ResponseWriterMock) WriteHeader(code int) { *r = ResponseWriterMock(code); }
func (r *ResponseWriterMock) GetCode() int { return int(*r); }

func ExampleRegexpRouter() {
	w := new(ResponseWriterMock)
	type P struct {
		S string `rhttp:"s"`
	}

	f := func(w http.ResponseWriter, r *http.Request, i interface{}) {
		params := i.(*P)
		fmt.Printf("%s\n", params.S)
	}

	router := NewRouter()
	router.HandleFunc("/(s=[abc])", f, &P{})

	r, _ := http.NewRequest("GET", "/a", strings.NewReader(""))
	router.ServeHTTP(w, r)

	r, _ = http.NewRequest("GET", "/b", strings.NewReader(""))
	router.ServeHTTP(w, r)

	w.WriteHeader(0)
	r, _ = http.NewRequest("GET", "/d", strings.NewReader(""))
	router.ServeHTTP(w, r)
	fmt.Printf("%d\n", w.GetCode())

	// Output:
        // a
        // b
	// 404
}

func ExampleRegexpRouterMulti() {
	w := new(ResponseWriterMock)
	type P struct {
		S string `rhttp:"s"`
	}

	f := func (name string) func(w http.ResponseWriter, r *http.Request, i interface{}) {
		return func(w http.ResponseWriter, r *http.Request, i interface{}) {
			params := i.(*P)
			fmt.Printf("%s: %s\n", name, params.S)
		}
	}

	router := NewRouter()
	router.HandleFunc("/a(s=[12])(/?)", f("a1"), &P{})
	router.HandleFunc("/a(s=[123])", f("a2"), &P{})
	router.HandleFunc("/b(s=[123])", f("b"), &P{})

	r, _ := http.NewRequest("GET", "/a1", strings.NewReader(""))
	router.ServeHTTP(w, r)

	r, _ = http.NewRequest("GET", "/a2/", strings.NewReader(""))
	router.ServeHTTP(w, r)

	r, _ = http.NewRequest("GET", "/a3", strings.NewReader(""))
	router.ServeHTTP(w, r)

	r, _ = http.NewRequest("GET", "/b1", strings.NewReader(""))
	router.ServeHTTP(w, r)

	// Output:
	// a1: 1
	// a1: 2
	// a2: 3
	// b: 1
}

func ExampleRegexpRouterNested() {
	w := new(ResponseWriterMock)
	type P struct {
		S1 string `rhttp:"s1"`
		S2 string `rhttp:"s2"`
	}

	f := func(w http.ResponseWriter, r *http.Request, i interface{}) {
		params := i.(*P)
		fmt.Printf("%s %s\n", params.S1, params.S2)
	}

	router := NewRouter()
	router.HandleFunc("/a(s1=(ab(c*))*)/b(s2=(ba)*)", f, &P{})

	r, _ := http.NewRequest("GET", "/aabcabccc/bbaba", strings.NewReader(""))
	router.ServeHTTP(w, r)

	// Output:
	// abcabccc baba
}

func ExampleRegexpRouterTyped() {
	w := new(ResponseWriterMock)
	type P1 struct {
		V1 int
		V2 int
	}

	type P2 struct {
		V1 int
		V2 string
	}

	f1 := func(w http.ResponseWriter, r *http.Request, i interface{}) {
		params := i.(*P1)
		fmt.Printf("f1: %d %d\n", params.V1, params.V2)
	}

	f2 := func(w http.ResponseWriter, r *http.Request, i interface{}) {
		params := i.(*P2)
		fmt.Printf("f2: %d '%s'\n", params.V1, params.V2)
	}

	router := NewRouter()
	router.HandleFunc("/(V1=.*)/(V2=.*)", f1, &P1{})
	router.HandleFunc("/(V1=.*)/(V2=.*)", f2, &P2{})

	r, _ := http.NewRequest("GET", "/123/123", strings.NewReader(""))
	router.ServeHTTP(w, r)

	r, _ = http.NewRequest("GET", "/123/abc", strings.NewReader(""))
	router.ServeHTTP(w, r)

	w.WriteHeader(0)
	r, _ = http.NewRequest("GET", "/123/", strings.NewReader(""))
	router.ServeHTTP(w, r)

	w.WriteHeader(0)
	r, _ = http.NewRequest("GET", "//abc", strings.NewReader(""))
	router.ServeHTTP(w, r)
	fmt.Printf("%d\n", w.GetCode())

	// Output:
	// f1: 123 123
	// f2: 123 'abc'
	// f2: 123 ''
	// 404
}

// This is vital for ensuring that ambiguous routes our handled in the order added
func TestRegexpRouterRoutesSorting (t *testing.T) {
	w := new(ResponseWriterMock)

	h := 0
	f1  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  1 }
	f2  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  2 }
	f3  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  3 }
	f4  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  4 }
	f5  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  5 }
	f6  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  6 }
	f7  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  7 }
	f8  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  8 }
	f9  := func(w http.ResponseWriter, r *http.Request, i interface{}) { h =  9 }
	f10 := func(w http.ResponseWriter, r *http.Request, i interface{}) { h = 10 }

	router := NewRouter()
	router.HandleFunc("/b/c", f9, nil)
	router.HandleFunc("/b/c(/?)", f10, nil)
	router.HandleFunc("/b", f7, nil)
	router.HandleFunc("/b(/?)", f8, nil)
	router.HandleFunc("/a/b/c", f5, nil)
	router.HandleFunc("/a/b/c(/?)", f6, nil)
	router.HandleFunc("/a/b", f3, nil)
	router.HandleFunc("/a/b(/?)", f4, nil)
	router.HandleFunc("/a", f1, nil)
	router.HandleFunc("/a(/?)", f2, nil)

	h = 0
	r, _ := http.NewRequest("GET", "/a", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 1 { t.Fatalf("Expected handler %d, got %d", 1, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/a/", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 2 { t.Fatalf("Expected handler %d, got %d", 2, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/a/b", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 3 { t.Fatalf("Expected handler %d, got %d", 3, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/a/b/", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 4 { t.Fatalf("Expected handler %d, got %d", 4, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/a/b/c", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 5 { t.Fatalf("Expected handler %d, got %d", 5, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/a/b/c/", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 6 { t.Fatalf("Expected handler %d, got %d", 6, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/b", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 7 { t.Fatalf("Expected handler %d, got %d", 7, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/b/", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 8 { t.Fatalf("Expected handler %d, got %d", 8, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/b/c", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 9 { t.Fatalf("Expected handler %d, got %d", 9, h) }

	h = 0
	r, _ = http.NewRequest("GET", "/b/c/", strings.NewReader(""))
	router.ServeHTTP(w, r)
	if h != 10 { t.Fatalf("Expected handler %d, got %d", 10, h) }
}

func TestCompilePatternSimple(t *testing.T) {
	pattern := "/(var=.*)/static"
	nameToField := map[string]int{"var": 1}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/(.*)/static$")
	exPrefix := "/"
	exParamsIndex := map[int]int{1: 0}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternNone(t *testing.T) {
	pattern := "/static"
	nameToField := map[string]int{}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/static$")
	exPrefix := "/static"
	exParamsIndex := map[int]int{}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternMulti(t *testing.T) {
	pattern := "/prefix/(v1=.*)/(v2=\\d*)/(v3=123?)"
	nameToField := map[string]int{"v1": 2, "v2": 1, "v3": 3}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/prefix/(.*)/(\\d*)/(123?)$")
	exPrefix := "/prefix/"
	exParamsIndex := map[int]int{2: 0, 1: 1, 3: 2}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternAnon(t *testing.T) {
	pattern := "/prefix(/?)"
	nameToField := map[string]int{}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/prefix(/?)$")
	exPrefix := "/prefix"
	exParamsIndex := map[int]int{}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternAnonMix(t *testing.T) {
	pattern := "/prefix/(v1=.*)/(/?)"
	nameToField := map[string]int{"v1": 1}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/prefix/(.*)/(/?)$")
	exPrefix := "/prefix/"
	exParamsIndex := map[int]int{1: 0}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternAbutting(t *testing.T) {
	pattern := "/prefix/(v1=.*)(/?)"
	nameToField := map[string]int{"v1": 1}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/prefix/(.*)(/?)$")
	exPrefix := "/prefix/"
	exParamsIndex := map[int]int{1: 0}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternEscaping(t *testing.T) {
	pattern := "/prefix\\(\\)/(v1=.*)/\\(\\)/"
	nameToField := map[string]int{"v1": 1}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/prefix\\(\\)/(.*)/\\(\\)/$")
	exPrefix := "/prefix()/"
	exParamsIndex := map[int]int{1: 0}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternWithEquals(t *testing.T) {
	pattern := "/prefix/(v1==)/"
	nameToField := map[string]int{"v1": 1}

	re, prefix, paramsIndex, err := compilePattern(pattern, nameToField)
	exRe := regexp.MustCompile("^/prefix/(=)/$")
	exPrefix := "/prefix/"
	exParamsIndex := map[int]int{1: 0}
	assertCompilePattern(t, re, exRe, prefix, exPrefix, paramsIndex, exParamsIndex, err)
}

func TestCompilePatternUnterminatedFailure(t *testing.T) {
	pattern := "/prefix/(v1="
	nameToField := map[string]int{"v1": 1}

	_, _, _, err := compilePattern(pattern, nameToField)
	assertCompilePatternFailure(t, err, Unterminated, "Pattern '/prefix/(v1=' contains unterminated group.")
}

func TestCompilePatternMissingVariableNameFailure(t *testing.T) {
	pattern := "/prefix/(=abc)"
	nameToField := map[string]int{"v1": 1}

	_, _, _, err := compilePattern(pattern, nameToField)
	assertCompilePatternFailure(t, err, MissingVariableName,
		"Expected variable name in group '(=abc)' of '/prefix/(=abc)' at 9")
}

func TestCompilePatternInvalidVariableNameFailure(t *testing.T) {
	pattern := "/prefix/(()=abc)"
	nameToField := map[string]int{"v1": 1}

	_, _, _, err := compilePattern(pattern, nameToField)
	assertCompilePatternFailure(t, err, InvalidVariableName,
		"Invalid variable name '()' in group '(()=abc)' of '/prefix/(()=abc)' at 9.")
}

func TestCompilePatternUndefinedVariableFailure(t *testing.T) {
	pattern := "/prefix/(v2=abc)"
	nameToField := map[string]int{"v1": 1}

	_, _, _, err := compilePattern(pattern, nameToField)
	assertCompilePatternFailure(t, err, UndefinedVariable,
		"Undefined variable 'v2' in group '(v2=abc)' of '/prefix/(v2=abc)' at 9.")
}

func TestCompilePatternUnmatchedRightParen(t *testing.T) {
	pattern := "/prefix/)(v2=abc)"
	nameToField := map[string]int{"v1": 1}

	_, _, _, err := compilePattern(pattern, nameToField)
	assertCompilePatternFailure(t, err, UnmatchedRightParen,
		"Unmatched '(' of '/prefix/)(v2=abc)' at 8.")
}

func TestReadParamsNil(t *testing.T) {
	nameToField, _, err := readParams(nil)
	exNameToField := map[string]int{}
	assertReadParams(t, nameToField, exNameToField, err)
}

func TestReadParamsTags(t *testing.T) {
	var params struct {
		F float64 `rhttp:"v1"`
		U uint `rhttp:"v2"`
		S string `rhttp:"v3"`
		I int `rhttp:"v4"`
	}
	nameToField, _, err := readParams(&params)
	exNameToField := map[string]int{"v1": 0, "v2": 1, "v3": 2, "v4": 3}
	assertReadParams(t, nameToField, exNameToField, err)
}

func TestReadParamsFields(t *testing.T) {
	var params struct {
		F float64
		U uint
		S string
		I int
	}
	nameToField, _, err := readParams(&params)
	exNameToField := map[string]int{"F": 0, "U": 1, "S": 2, "I": 3}
	assertReadParams(t, nameToField, exNameToField, err)
}

func TestReadParamsMix(t *testing.T) {
	var params struct {
		F float64 `rhttp:"v1"`
		U uint `rhttp:"v2"`
		S string
		I int
	}
	nameToField, _, err := readParams(&params)
	exNameToField := map[string]int{"v1": 0, "v2": 1, "S": 2, "I": 3}
	assertReadParams(t, nameToField, exNameToField, err)
}

func TestReadParamsOverwrite(t *testing.T) {
	var params struct {
		V1 float64
		V2 uint `rhttp:"V1"`
	}
	nameToField, _, err := readParams(&params)
	exNameToField := map[string]int{"V1": 1}
	assertReadParams(t, nameToField, exNameToField, err)
}

func TestReadParamsNoOverwrite(t *testing.T) {
	var params struct {
		V1 float64 `rhttp:"V2"`
		V2 uint
	}
	nameToField, _, err := readParams(&params)
	exNameToField := map[string]int{"V2": 0}
	assertReadParams(t, nameToField, exNameToField, err)
}

func TestReadParamsInvalidNameFailure(t *testing.T) {
	var params struct {
		V1 []int `rhttp:"v1"`
	}
	_, _, err := readParams(&params)
	if err == nil {
		t.FailNow()
	}
}

func TestReadParamsInvalidTypeFailure(t *testing.T) {
	var params struct {
		V1 []int `rhttp:"v1"`
	}
	_, _, err := readParams(&params)
	if err == nil {
		t.FailNow()
	}
}

func TestReadParamsInvalidTypeUntaggedIgnored(t *testing.T) {
	var params struct {
		V1 []int
	}
	_, _, err := readParams(&params)
	if err != nil {
		t.Fatalf("Unexpected error `%v`\n", err)
	}
}

func TestReadParamsInvalidInterfaceFailure(t *testing.T) {
	var params struct {
		V1 []int
	}
	_, _, err := readParams(params)
	if err == nil {
		t.FailNow()
	}
}

func assertCompilePattern(t *testing.T, re, exRe *regexp.Regexp, prefix, exPrefix string,
	paramsIndex, exParamsIndex map[int]int, err error) {

	if err != nil {
		t.Fatalf("unexpected error: `%v`\n", err)
	}
	if exRe == nil && re != nil || re == nil && exRe != nil || re.String() != exRe.String() {
		t.Fatalf("expected regexp `%v`, got: `%v`\n", exRe, re)
	}
	if prefix != exPrefix {
		t.Fatalf("expected prefix `%v`, got: `%v`\n", exPrefix, prefix)
	}
	if !intIntMapEq(paramsIndex, exParamsIndex) {
		t.Fatalf("expected paramsIndex `%v`, got: `%v'\n", exParamsIndex, paramsIndex)
	}
}

func assertCompilePatternFailure(t *testing.T, err error, cause errorCause, formattedErr string) {
	if err == nil {
		t.Fatalf("Expected error `%v`\n", cause)
	}
	switch compileErr := err.(type) {
	case *PatternCompileError:
		if compileErr.cause != cause {
			t.Fatalf("Expected error `%v`, got `%v`\n", cause, compileErr.cause)
		}
		if strings.Index(err.Error(), formattedErr) == -1 {
			t.Fatalf("Expected error text containing `%s` got `%v`\n", formattedErr, err)
		}
	}
}

func assertReadParams(t *testing.T, nameToField, exNameToField map[string]int, err error) {
	if err != nil {
		t.Fatalf("Unexpected error `%v`\n", err)
	}
	if !strIntMapEq(nameToField, exNameToField) {
		t.Fatalf("Expected nameToField `%v`, got `%v`\n", nameToField, exNameToField)
	}
}

func intIntMapEq(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			return false
		}
	}
	return true
}

func strIntMapEq(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			return false
		}
	}
	return true
}

