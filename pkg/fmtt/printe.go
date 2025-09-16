package fmtt

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/davecgh/go-spew/spew"
)

// PrintErrChain walks an error chain and prints each layer with its type.
func PrintErrChain(err error) {
	if err == nil {
		fmt.Println("<nil>")
		return
	}

	i := 0
	for e := err; e != nil; e = errors.Unwrap(e) {
		fmt.Printf("[%d] %T: %v\n", i, e, e)
		i++
	}
}

func PrintErrChainDebug(err error) {
	for i := 0; err != nil; err = errors.Unwrap(err) {
		fmt.Printf("[%d] %T\n", i, err)
		fmt.Printf("   Error(): %v\n", err)

		// Dump with spew
		spew.Dump(err)

		// Reflect struct fields
		rv := reflect.ValueOf(err)
		rt := reflect.TypeOf(err)
		if rt.Kind() == reflect.Ptr {
			rv = rv.Elem()
			rt = rt.Elem()
		}
		if rt.Kind() == reflect.Struct {
			for j := 0; j < rt.NumField(); j++ {
				f := rt.Field(j)
				v := rv.Field(j)
				if v.CanInterface() {
					fmt.Printf("   Field %s (%s): %+v\n", f.Name, f.Type, v.Interface())
				}
			}
		}

		// Common interfaces
		if u, ok := err.(interface{ Unwrap() error }); ok {
			fmt.Printf("   Has Unwrap(): %T\n", u.Unwrap())
		}
		if c, ok := err.(interface{ Cause() error }); ok {
			fmt.Printf("   Has Cause(): %T\n", c.Cause())
		}

		i++
	}
}
