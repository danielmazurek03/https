// Copyright 2013 Carl Chatfield (carlchatfield@gmail.com). All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rhttp provides a means for routing http requests from the http
// package to handlers registered to match a particular regular expression.
// The routing  mechanism is fully compatible with the http package and is
// designed to complement, not replace it.
//
// Routing
//
// RegexpRouter implements http.Handler and can therefore be registered
// on a pattern as any other handler. However, RegexpRouter also exposes
// methods similar to http.ServeMux allowing a rhttp.Handler to be registered
// on a regular expression. For example:
//
//     type DoParams struct {
//         Var string `rhttp:"var"`
//     }
//     func Do(w http.ResponseWriter, r *http.Request, i interface{}) {
//         params := i.(*DoParams)
//         w.Write([]byte(params.Var))
//     }
//     func main() {
//         router := rhttp.NewRouter()
//         router.HandleFunc("/re/(var=.*)", Do, &DoParams{})
//         http.Handle("/re/", router)
//         ...
//     }
//
// A simple syntax for defining routes is used where a variable name is followed
// by an '=' and a regular expression, all surrounded by parenthesis. For example:
//
//     /static/(var_name=re)/static(/?)
//
// The variable name is optional; its omission results in an anonymous group
// which is not represented in the parameters passed back to the Handler. This
// may be useful for matching on a path whether or not the trailing '/' is
// present. Note that as a consequence of the syntax, an anonymous group may
// not contain and '=' in its regular expression. As a work around, a dummy
// variable will need to be used.
//
// Variable naming follows the same rules as go.
//
// Parts of the pattern outside the groups are not parsed as regular
// expressions.  If needed, these parts may contain escape sequences \(, \\, or
// \) to insert the corresponding character into the path. Variable names may
// not be escaped, and regular expressions obey the usual escaping rules.
//
// One important difference between http and rhttp is the precedence of routes.
// http uses a longest wins approach. This doesn't make sense for patterns
// containing regular expressions. In fact, two very different regular
// expressions could potentially both match a path.
//
// Like http, rhttp also uses longest wins at the prefix level. The prefix is
// from the beginning of the pattern until the first group. If multiple patterns
// have the same prefix, the earliest registered pattern will take precedence
// and be tested first. If the pattern fails, the next earliest registered
// pattern with the same prefix will be tried, and so forth until all registered
// patterns have been exhausted.
//
// Variable Resolution
//
// Reflection is used to determine a scope variables appearing in the pattern.
// At registration, an otherwise unused struct is provided as a final argument. 
// The router will traverse the fields of the struct searching for rhttp tags:
//
//     struct {
//         Var string `rhttp:"var"`
//     }
//
// Only exported fields (those beginning with a capital letter) can be used.
// Tagged fields must be of type int, uint, float or string, and the string
// inside the tag must be a valid variable name as described above. Failure
// to meet these conditions will result in a run time panic.
//
// Untagged fields are also considered if they are of an allowed type and the
// name does not conflict with a manually tagged field.
// 
// Any variable names used in a pattern must present in the scope defined by
// the struct. The variables are also typed. If a pattern is matched but does
// not type match, the route is not taken.

package rhttp

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"strconv"

	"net/http"
)

type PatternCompileError struct {
	cause errorCause
	currentGroup string
	pattern string
	name string
	at int
}

type errorCause int
const (
	Unterminated errorCause = iota
	MissingVariableName
	InvalidVariableName
	UndefinedVariable
	UnmatchedRightParen
)

// Analogous to http.Handler, ServeHTTP will be called with an
// empty interface which can be cast to the type the Handler
// was registered with.
type Handler interface {
        ServeHTTP(http.ResponseWriter, *http.Request, interface{})
}

type HandlerFunc func(http.ResponseWriter, *http.Request, interface{})

// A http.Handler that will take the usual http request and response
// and re-route it to the appropriate rhttp.Handler.
type RegexpRouter struct {
	routes []*regexpHandler
}

type regexpHandler struct {
	handler Handler
	prefix string
	re *regexp.Regexp
	paramsType reflect.Type
	// Indexes the position of a variable in the url to a field on the params struct
	paramsIndex map[int]int
}

// Create a new Router
func NewRouter() *RegexpRouter {
	return &RegexpRouter{}
}

// Registers a Handler with the RegexpRouter.
func (r *RegexpRouter) Handle(pattern string, handler Handler, params interface{}) {
	rh := &regexpHandler{handler: handler}
	if nameToField, paramsType, err := readParams(params); err != nil {
		panic(err)
	} else if rh.re, rh.prefix, rh.paramsIndex, err = compilePattern(pattern, nameToField); err != nil {
		panic(err)
	} else {
		rh.paramsType = paramsType
		// Insert into the list of routes *before* all others with the same prefix
		// The result is that routes are alphabetically sorted, but routes with the
		// same prefix will be in reverse.
		// When routing we will work through the list backwards.
		i := sort.Search(len(r.routes), func(i int) bool { return r.routes[i].prefix >= rh.prefix })
		r.routes = append(r.routes[:i], append([]*regexpHandler{rh}, r.routes[i:]...)...)
	}
}

// An adapter allowing an appropriate function to also be registered.
func (r *RegexpRouter) HandleFunc(pattern string, f func(http.ResponseWriter, *http.Request, interface{}), params interface{}) {
	r.Handle(pattern, HandlerFunc(f), params)
}

// Reroute the request to the appropriate rhttp.Handler using req.Path
func (r *RegexpRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Find the *last* + 1 matching prefix and then proceed *backwards* through them.
	path := req.URL.Path
	h := sort.Search(len(r.routes), func(i int) bool {
		return r.routes[i].prefix > path
	})

	// Go through each handler in order added until we get a hit. Remember, they are backwards
keepLooking:
	// Note[crc] This decrement must be placed after keepLooking, as it is no use
	// coming back to only find the same route again. It happens I got lucky and
	// the usual code path requires the decrement anyway.
	h -= 1 // We went past the end of the matching prefixes, backtrack one
	is := []int{}
	for ; h >= 0 && strings.HasPrefix(path, r.routes[h].prefix); h -= 1 {
		if is = r.routes[h].re.FindStringSubmatchIndex(path); len(is) != 0 {
			break;
		}
	}

	if len(is) == 0 {
		http.NotFound(w, req); return
	}

	// If no params we're registered on this route, there is no need to do anything more.
	if r.routes[h].paramsType == nil {
		r.routes[h].handler.ServeHTTP(w, req, nil)
		return
	}

	// extract the variable values in the outer most submatches of the regexp
	var vals []string
	matchEnd := 0
	for i := 2; i < len(is); i += 2 {
		if is[i] >= matchEnd {
			vals = append(vals, path[is[i]:is[i+1]])
			matchEnd = is[i+1]
		}
	}

	// Create the params, checking types. If the types don't go back and keep looking
	params := reflect.New(r.routes[h].paramsType)
	for f, v := range r.routes[h].paramsIndex {
		value := params.Elem().Field(f)
		switch value.Type().Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if val, err := strconv.ParseInt(vals[v], 10, 64); err != nil {
				goto keepLooking;
			} else {
				value.SetInt(val)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if val, err := strconv.ParseUint(vals[v], 10, 64); err != nil {
				goto keepLooking;
			} else {
				value.SetUint(val)
			}
		case reflect.Float32, reflect.Float64:
			if val, err := strconv.ParseFloat(vals[v], 64); err != nil {
				goto keepLooking;
			} else {
				value.SetFloat(val)
			}
		case reflect.String:
			value.SetString(vals[v])
		default:
			// Should be unreachable as readParams only returns the above kinds
			panic(fmt.Sprintf("Cannot set type %s", value.Kind()))
		}
	}

	r.routes[h].handler.ServeHTTP(w, req, params.Interface())
}

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request, params interface{}) {
	f(w, r, params)
}

func compilePattern(pattern string, nameToField map[string]int) (*regexp.Regexp, string, map[int]int, error) {
	var err *PatternCompileError
	var prefix string
	paramsIndex := make(map[int]int)
	paramIndex := 0
	groupBegin := 0
	groupEnd := 0
	equals := groupBegin
	parens := 0
	regexpString := ""

	// Only regions outside matches may be escaped
	escape := false
	for i, r := range pattern {
		if escape {
			escape = false
		} else {
			if parens == 0 {
				if r == '\\' {
					escape = true
				} else if r == '(' {
					parens += 1
					// Include the ( in the copied pattern
					piece := removeEscapes(pattern[groupEnd:i])
					regexpString += regexp.QuoteMeta(piece) + "("
					if groupBegin == 0 {
						prefix = piece
					}
					groupBegin = i
					// If there is no =, then the regex is anonymous and the
					// entire group should be copied into the final regexp
					equals = groupBegin
				} else if r == ')' {
					return nil, "", paramsIndex, &PatternCompileError{
						cause: UnmatchedRightParen,
						pattern: pattern,
						at: i}
				}
			} else {
				if r == '(' {
					parens += 1
				} else if r == ')' {
					parens -= 1
					if parens == 0 {
						groupEnd = i+1
						// Check for a pending error. Now that we have
						// read the entire group, we can give a
						// meaningful error message.
						if err != nil {
							err.currentGroup = pattern[groupBegin:groupEnd]
							err.pattern = pattern
							return nil, "", paramsIndex, err
						}
						// Include the ) in the copied pattern, but not the =
						regexpString += pattern[equals+1:i+1]
					}
				} else if r == '=' && equals == groupBegin {
					name := pattern[groupBegin+1:i]
					if len(name) == 0 {
						err = &PatternCompileError{
							cause: MissingVariableName,
							at: groupBegin + 1}
					} else if !allowedName(name) {
						err = &PatternCompileError{
							cause: InvalidVariableName,
							name: name,
							at: groupBegin + 1}
					} else if _, ok := nameToField[name]; !ok {
						err = &PatternCompileError{
							cause: UndefinedVariable,
							name: name,
							at: groupBegin + 1}
					}
					paramsIndex[nameToField[name]] = paramIndex
					paramIndex += 1
					equals = i
				}
			}
		}
	}
	if parens != 0 {
		return nil, "", paramsIndex, &PatternCompileError{
			cause: Unterminated,
			pattern: pattern,
			at: groupBegin}
	}

	// Add any remaining part of the pattern outside the groups
	piece := removeEscapes(pattern[groupEnd:])
	regexpString += regexp.QuoteMeta(piece)
	if groupBegin == 0 {
		prefix = piece
	}
	r, reErr := regexp.Compile("^" + regexpString + "$")
	return r, prefix, paramsIndex, reErr
}

// Use reflection to map variable names to an index on the params struct
func readParams(params interface{}) (nameToField map[string]int, paramsType reflect.Type, err error) {
	nameToField = make(map[string]int)
	v := reflect.ValueOf(params)
	if params == nil || v.Kind() == reflect.Ptr && v.IsNil() {
		return
	}
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	} else {
		err = errors.New(fmt.Sprintf("params not a struct pointer (%s)", v.Kind()))
		return
	}

	if v.Kind() != reflect.Struct {
		err = errors.New(fmt.Sprintf("*params is not a struct (%s)", v.Kind()))
		return
	}
	paramsType = v.Type()
	// Parse struct tags and use user specified variable name
	for i := 0; i < v.NumField(); i += 1 {
		f := v.Type().Field(i)
		tag := f.Tag.Get("rhttp")
		if len(tag) != 0 {
			if !allowedName(tag) {
				err = errors.New(fmt.Sprintf("Invalid param name '%s' (%s %s %s)",
					tag, f.Name, f.Type.Kind(), f.Tag))
				return
			}
			if !allowedType(f.Type.Kind()) {
				err = errors.New(fmt.Sprintf("Invalid param type '%s' (%s %s %s)",
					f.Type.Kind(), f.Name, f.Type.Kind(), f.Tag))
				return
			}
			nameToField[tag] = i
		} else {
			// Untagged fields are assigned by name with lower precedence.
			// Unlike tagged fields, we simply ignore incompatible fields
			if allowedType(f.Type.Kind()) && allowedName(f.Name) {
				// Do not overwrite tagged fields
				if _, ok := nameToField[f.Name]; !ok {
					nameToField[f.Name] = i
				}
			}
		}
	}
	return nameToField, paramsType, nil
}

func allowedType(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	}
	return false
}

var allowedNameRe = regexp.MustCompile("^[a-zA-Z_]\\w*$")
func allowedName(name string) bool {
	return allowedNameRe.MatchString(name)
}

func (err* PatternCompileError) Error() string {
	switch err.cause {
	case Unterminated:
		return fmt.Sprintf("Pattern '%s' contains unterminated group.", err.pattern)
	case MissingVariableName:
		return fmt.Sprintf("Expected variable name in group '%s' of '%s' at %d.",
			err.currentGroup, err.pattern, err.at)
	case InvalidVariableName:
		return fmt.Sprintf("Invalid variable name '%s' in group '%s' of '%s' at %d.",
			err.name, err.currentGroup, err.pattern, err.at)
	case UndefinedVariable:
		return fmt.Sprintf("Undefined variable '%s' in group '%s' of '%s' at %d.",
			err.name, err.currentGroup, err.pattern, err.at)
	case UnmatchedRightParen:
		return fmt.Sprintf("Unmatched '(' of '%s' at %d.", err.pattern, err.at)
	default:
		panic(fmt.Sprintf("Encountered undefined error %s", err.cause))
	}
}

func removeEscapes(s string) string {
	unescaped := ""
	begin := 0
	for i := 0; i < len(s); i += 1 {
		if s[i] == '\\' {
			unescaped += s[begin:i]
			i += 1
			begin = i
		}
	}
	unescaped += s[begin:]
	return unescaped
}
