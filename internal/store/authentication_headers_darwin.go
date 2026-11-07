package store

import (
	"errors"
	"runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var loadNativeStore = sync.OnceValue(func() error {
	// Classes and singleton objects outlive requests; retain the framework handle.
	_, err := purego.Dlopen("/System/Library/PrivateFrameworks/AppleMediaServices.framework/AppleMediaServices", purego.RTLD_NOW|purego.RTLD_LOCAL)
	return err
})

func platformAuthenticationHeaders(endpoint string) (map[string]string, error) {
	if err := loadNativeStore(); err != nil {
		return nil, errors.New("macOS Store framework is unavailable")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))

	class := objc.ID(objc.GetClass("AMSAnisette"))
	selector := objc.RegisterName("_headersForRequest:account:type:")
	if class == 0 || !objc.Send[bool](class, objc.RegisterName("respondsToSelector:"), selector) {
		return nil, errors.New("macOS Store device proof is unavailable")
	}
	stringClass := objc.ID(objc.GetClass("NSString"))
	stringSelector := objc.RegisterName("stringWithUTF8String:")
	url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("URLWithString:"), stringClass.Send(stringSelector, endpoint))
	request := objc.ID(objc.GetClass("NSURLRequest")).Send(objc.RegisterName("requestWithURL:"), url)
	// Type 2 is the Store AMD proof, distinct from AuthKit's I-MD anisette.
	values := class.Send(selector, request, objc.ID(0), int64(2))
	if values == 0 || !objc.Send[bool](values, objc.RegisterName("isKindOfClass:"), objc.GetClass("NSDictionary")) {
		return nil, errors.New("macOS returned no Store device proof")
	}
	headers := make(map[string]string, 2)
	for _, key := range []string{"X-Apple-AMD", "X-Apple-AMD-M"} {
		value := values.Send(objc.RegisterName("objectForKey:"), stringClass.Send(stringSelector, key))
		if value == 0 || !objc.Send[bool](value, objc.RegisterName("isKindOfClass:"), objc.GetClass("NSString")) {
			return nil, errors.New("macOS returned an incomplete Store device proof")
		}
		length := objc.Send[uint64](value, objc.RegisterName("lengthOfBytesUsingEncoding:"), uint64(4))
		if length == 0 || length > 8192 {
			return nil, errors.New("macOS returned an invalid Store device proof")
		}
		text := objc.Send[string](value, objc.RegisterName("UTF8String"))
		if strings.ContainsAny(text, "\r\n") {
			return nil, errors.New("macOS returned an invalid Store device proof")
		}
		headers[key] = text
	}
	return headers, nil
}
