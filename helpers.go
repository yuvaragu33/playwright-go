package playwright

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/danwakefield/fnmatch"
)

type (
	routeHandler = func(*Route, *Request)
)

func skipFieldSerialization(val reflect.Value) bool {
	typ := val.Type()
	return (typ.Kind() == reflect.Ptr ||
		typ.Kind() == reflect.Interface ||
		typ.Kind() == reflect.Map ||
		typ.Kind() == reflect.Slice) && val.IsNil()
}

func transformStructIntoMapIfNeeded(inStruct interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	v := reflect.ValueOf(inStruct)
	typ := v.Type()
	if v.Kind() == reflect.Struct {
		// Merge into the base map by the JSON struct tag
		for i := 0; i < v.NumField(); i++ {
			fi := typ.Field(i)
			// Skip the values when the field is a pointer (like *string) and nil.
			if !skipFieldSerialization(v.Field(i)) {
				// We use the JSON struct fields for getting the original names
				// out of the field.
				tagv := fi.Tag.Get("json")
				key := strings.Split(tagv, ",")[0]
				if key == "" {
					key = fi.Name
				}
				out[key] = v.Field(i).Interface()
			}
		}
	} else if v.Kind() == reflect.Map {
		// Merge into the base map
		for _, key := range v.MapKeys() {
			if !skipFieldSerialization(v.MapIndex(key)) {
				out[key.String()] = v.MapIndex(key).Interface()
			}
		}
	}
	return out
}

// transformOptions handles the parameter data transformation
func transformOptions(options ...interface{}) map[string]interface{} {
	var base map[string]interface{}
	var option interface{}
	// Case 1: No options are given
	if len(options) == 0 {
		return make(map[string]interface{})
	}
	if len(options) == 1 {
		// Case 2: a single value (either struct or map) is given.
		base = make(map[string]interface{})
		option = options[0]
	} else if len(options) == 2 {
		// Case 3: two values are given. The first one needs to be a map and the
		// second one can be a struct or map. It will be then get merged into the first
		// base map.
		base = transformStructIntoMapIfNeeded(options[0])
		option = options[1]
	}
	v := reflect.ValueOf(option)
	if v.Kind() == reflect.Slice {
		if v.Len() == 0 {
			return base
		}
		option = v.Index(0).Interface()
	}

	if option == nil {
		return base
	}
	v = reflect.ValueOf(option)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	optionMap := transformStructIntoMapIfNeeded(v.Interface())
	for key, value := range optionMap {
		base[key] = value
	}
	return base
}

func remapMapToStruct(ourMap interface{}, structPtr interface{}) {
	ourMapV := reflect.ValueOf(ourMap)
	structV := reflect.ValueOf(structPtr).Elem()
	structTyp := structV.Type()
	for i := 0; i < structV.NumField(); i++ {
		fi := structTyp.Field(i)
		tagv := fi.Tag.Get("json")
		key := strings.Split(tagv, ",")[0]
		for _, e := range ourMapV.MapKeys() {
			if key == e.String() {
				value := ourMapV.MapIndex(e).Interface()
				switch v := value.(type) {
				case int:
					structV.Field(i).SetInt(int64(v))
				case string:
					structV.Field(i).SetString(v)
				case bool:
					structV.Field(i).SetBool(v)
				default:
					panic(ourMapV.MapIndex(e).Kind())
				}
			}
		}
	}
}

func isFunctionBody(expression string) bool {
	expression = strings.TrimSpace(expression)
	return strings.HasPrefix(expression, "function") ||
		strings.HasPrefix(expression, "async ") ||
		strings.Contains(expression, "=> ")
}

type urlMatcher struct {
	urlOrPredicate interface{}
}

func newURLMatcher(urlOrPredicate interface{}) *urlMatcher {
	return &urlMatcher{
		urlOrPredicate: urlOrPredicate,
	}
}

func (u *urlMatcher) Match(url string) bool {
	switch v := u.urlOrPredicate.(type) {
	case *regexp.Regexp:
		return v.MatchString(url)
	case string:
		return fnmatch.Match(v, url, 0)
	}
	if reflect.TypeOf(u.urlOrPredicate).Kind() == reflect.Func {
		function := reflect.ValueOf(u.urlOrPredicate)
		result := function.Call([]reflect.Value{reflect.ValueOf(url)})
		return result[0].Bool()
	}
	panic(u.urlOrPredicate)
}

type routeHandlerEntry struct {
	matcher *urlMatcher
	handler routeHandler
}

func newRouteHandlerEntry(matcher *urlMatcher, handler routeHandler) *routeHandlerEntry {
	return &routeHandlerEntry{
		matcher: matcher,
		handler: handler,
	}
}
