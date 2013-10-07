rhttp
=====

Regexp routing for go http

Package rhttp provides a means for routing http requests from the http
package to handlers registered to match a particular regular expression.
The routing  mechanism is fully compatible with the http package and is
designed to complement, not replace it.

Routing
-------

RegexpRouter implements http.Handler and can therefore be registered
on a pattern as any other handler. However, RegexpRouter also exposes
methods similar to http.ServeMux allowing a rhttp.Handler to be registered
on a regular expression. For example:

```go
    type DoParams struct {
        Var string `rhttp:"var"`
    }
    func Do(w http.ResponseWriter, r *http.Request, i interface{}) {
        params := i.(*DoParams)
        w.Write([]byte(params.Var))
    }
    func main() {
        router := rhttp.NewRouter()
        router.HandleFunc("/re/(var=.*)", Do, &DoParams{})
        http.Handle("/re/", router)
        ...
    }
```

A simple syntax for defining routes is used where a variable name is followed
by an `=` and a regular expression, all surrounded by parenthesis. For example:

```
    /static/(var_name=re)/static(/?)
```

The variable name is optional; its omission results in an anonymous group
which is not represented in the parameters passed back to the Handler. This
may be useful for matching on a path whether or not the trailing `/` is
present. Note that as a consequence of the syntax, an anonymous group may
not contain and `=` in its regular expression. As a work around, a dummy
variable will need to be used.

Variable naming follows the same rules as go.

Parts of the pattern outside the groups are not parsed as regular
expressions.  If needed, these parts may contain escape sequences `\(`, `\\`, or
`\)` to insert the corresponding character into the path. Variable names may
not be escaped, and regular expressions obey the usual escaping rules.

One important difference between http and rhttp is the precedence of routes.
http uses a longest wins approach. This doesn't make sense for patterns
containing regular expressions. In fact, two very different regular
expressions could potentially both match a path.

Like http, rhttp also uses longest wins at the prefix level. The prefix is
from the beginning of the pattern until the first group. If multiple patterns
have the same prefix, the earliest registered pattern will take precedence
and be tested first. If the pattern fails, the next earliest registered
pattern with the same prefix will be tried, and so forth until all registered
patterns have been exhausted.

Variable Resolution
-------------------

Reflection is used to determine a scope variables appearing in the pattern.
At registration, an otherwise unused struct is provided as a final argument. 
The router will traverse the fields of the struct searching for rhttp tags:

```go
    struct {
        Var string `rhttp:"var"`
    }
```

Only exported fields (those beginning with a capital letter) can be used.
Tagged fields must be of `type int, uint, float or string`, and the string
inside the tag must be a valid variable name as described above. Failure
to meet these conditions will result in a run time panic.

Untagged fields are also considered if they are of an allowed type and the
name does not conflict with a manually tagged field.

Any variable names used in a pattern must present in the scope defined by
the struct. The variables are also typed. If a pattern is matched but does
not type match, the route is not taken.

