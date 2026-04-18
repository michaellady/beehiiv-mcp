//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security

#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

// Silence the deprecation warnings on the legacy SecKeychain* / SecAccess /
// SecTrustedApplication APIs. These are deprecated in favor of SecItem* +
// kSecUseDataProtectionKeychain, but the new APIs do NOT expose the
// trusted-application ACL that lets us pre-authorize our binary to read the
// item without a user prompt. We deliberately use the legacy file-based
// keychain (login.keychain-db) here.
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// kc_set creates or updates a generic password item. On create, the access
// control list is populated with the current running binary as the sole
// trusted application — subsequent reads from the same binary skip the
// macOS "Allow / Always Allow" prompt entirely.
//
// Returns 0 on success, or a non-zero OSStatus on failure.
static OSStatus kc_set(
    const char *service, UInt32 serviceLen,
    const char *account, UInt32 accountLen,
    const char *label,   UInt32 labelLen,
    const void *data,    UInt32 dataLen
) {
    // First try to find an existing item so we can update in place.
    SecKeychainItemRef existing = NULL;
    OSStatus st = SecKeychainFindGenericPassword(
        NULL,
        serviceLen, service,
        accountLen, account,
        NULL, NULL,
        &existing
    );
    if (st == errSecSuccess && existing != NULL) {
        st = SecKeychainItemModifyAttributesAndData(existing, NULL, dataLen, data);
        CFRelease(existing);
        return st;
    }
    if (st != errSecItemNotFound && st != errSecSuccess) {
        if (existing != NULL) CFRelease(existing);
        return st;
    }

    // Build a SecAccess whose trusted-apps list contains the current binary.
    // Passing NULL to SecTrustedApplicationCreateFromPath means "self".
    SecTrustedApplicationRef selfApp = NULL;
    st = SecTrustedApplicationCreateFromPath(NULL, &selfApp);
    if (st != errSecSuccess) {
        return st;
    }

    const void *apps[] = { (const void *)selfApp };
    CFArrayRef trusted = CFArrayCreate(NULL, apps, 1, &kCFTypeArrayCallBacks);

    CFStringRef descriptor = CFStringCreateWithBytes(
        NULL, (const UInt8 *)label, labelLen, kCFStringEncodingUTF8, false
    );

    SecAccessRef access = NULL;
    st = SecAccessCreate(descriptor, trusted, &access);

    CFRelease(descriptor);
    CFRelease(trusted);
    CFRelease(selfApp);

    if (st != errSecSuccess) {
        return st;
    }

    // Build the attribute list: service, account, label.
    SecKeychainAttribute attrs[3] = {
        { kSecServiceItemAttr, serviceLen, (void *)service },
        { kSecAccountItemAttr, accountLen, (void *)account },
        { kSecLabelItemAttr,   labelLen,   (void *)label   },
    };
    SecKeychainAttributeList attrList = { 3, attrs };

    SecKeychainItemRef newItem = NULL;
    st = SecKeychainItemCreateFromContent(
        kSecGenericPasswordItemClass,
        &attrList,
        dataLen, data,
        NULL,       // default keychain (login)
        access,
        &newItem
    );
    CFRelease(access);
    if (newItem != NULL) CFRelease(newItem);
    return st;
}

// kc_get reads an item. On success, *outData is malloc'd (caller must free via
// SecKeychainItemFreeContent) and *outLen is set.
static OSStatus kc_get(
    const char *service, UInt32 serviceLen,
    const char *account, UInt32 accountLen,
    void **outData, UInt32 *outLen
) {
    return SecKeychainFindGenericPassword(
        NULL,
        serviceLen, service,
        accountLen, account,
        outLen, outData,
        NULL
    );
}

// kc_free frees the buffer returned by kc_get.
static void kc_free(void *data) {
    SecKeychainItemFreeContent(NULL, data);
}

// kc_delete removes an item. Returns errSecSuccess if removed or
// errSecItemNotFound if it was already absent.
static OSStatus kc_delete(
    const char *service, UInt32 serviceLen,
    const char *account, UInt32 accountLen
) {
    SecKeychainItemRef item = NULL;
    OSStatus st = SecKeychainFindGenericPassword(
        NULL,
        serviceLen, service,
        accountLen, account,
        NULL, NULL,
        &item
    );
    if (st != errSecSuccess) {
        return st;
    }
    st = SecKeychainItemDelete(item);
    CFRelease(item);
    return st;
}

// kc_errmsg returns a malloc'd UTF-8 string describing an OSStatus (or NULL).
// Caller must free with free().
static char *kc_errmsg(OSStatus status) {
    CFStringRef msg = SecCopyErrorMessageString(status, NULL);
    if (msg == NULL) return NULL;

    CFIndex len = CFStringGetLength(msg);
    CFIndex max = CFStringGetMaximumSizeForEncoding(len, kCFStringEncodingUTF8) + 1;
    char *buf = (char *)malloc(max);
    if (buf == NULL) {
        CFRelease(msg);
        return NULL;
    }
    if (!CFStringGetCString(msg, buf, max, kCFStringEncodingUTF8)) {
        buf[0] = '\0';
    }
    CFRelease(msg);
    return buf;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// macOSKeychain is a credStore backed by the legacy macOS login keychain,
// with an ACL that pre-authorizes the current binary so reads from it never
// prompt the user. Built with pure cgo against Security.framework — no
// third-party keychain library dependency.
type macOSKeychain struct{}

func newMacOSKeychain() credStore { return macOSKeychain{} }

// errSecItemNotFound mirrors the OSStatus constant; callers compare against it
// to distinguish "absent" from "broken".
const errSecItemNotFound = -25300

func (macOSKeychain) Set(service, account string, data []byte) error {
	if len(data) == 0 {
		return errors.New("keychain: refusing to store empty value")
	}
	label := "beehiiv-mcp " + account

	servicePtr, serviceLen := bytePtr(service)
	accountPtr, accountLen := bytePtr(account)
	labelPtr, labelLen := bytePtr(label)
	dataPtr, dataLen := rawPtr(data)

	st := C.kc_set(
		servicePtr, C.UInt32(serviceLen),
		accountPtr, C.UInt32(accountLen),
		labelPtr, C.UInt32(labelLen),
		dataPtr, C.UInt32(dataLen),
	)
	if st != C.errSecSuccess {
		return osErr("keychain set", st)
	}
	return nil
}

func (macOSKeychain) Get(service, account string) ([]byte, error) {
	servicePtr, serviceLen := bytePtr(service)
	accountPtr, accountLen := bytePtr(account)

	var outData unsafe.Pointer
	var outLen C.UInt32
	st := C.kc_get(
		servicePtr, C.UInt32(serviceLen),
		accountPtr, C.UInt32(accountLen),
		&outData, &outLen,
	)
	if st == errSecItemNotFound {
		return nil, errNotFound
	}
	if st != C.errSecSuccess {
		return nil, osErr("keychain get", st)
	}
	defer C.kc_free(outData)

	// Copy the buffer into Go memory before it's freed.
	buf := C.GoBytes(outData, C.int(outLen))
	return buf, nil
}

func (macOSKeychain) Delete(service, account string) error {
	servicePtr, serviceLen := bytePtr(service)
	accountPtr, accountLen := bytePtr(account)

	st := C.kc_delete(
		servicePtr, C.UInt32(serviceLen),
		accountPtr, C.UInt32(accountLen),
	)
	if st == errSecSuccess || st == errSecItemNotFound {
		return nil
	}
	return osErr("keychain delete", st)
}

// errSecSuccess is 0; cgo exposes it inconveniently so we define our own alias.
const errSecSuccess C.OSStatus = 0

// bytePtr returns a C pointer + byte length for a Go string. The pointer is
// valid for the duration of the enclosing cgo call.
func bytePtr(s string) (*C.char, int) {
	if len(s) == 0 {
		return nil, 0
	}
	b := []byte(s)
	return (*C.char)(unsafe.Pointer(&b[0])), len(b)
}

// rawPtr returns an unsafe.Pointer to the data's first byte (or nil if empty)
// plus its length.
func rawPtr(data []byte) (unsafe.Pointer, int) {
	if len(data) == 0 {
		return nil, 0
	}
	return unsafe.Pointer(&data[0]), len(data)
}

// osErr turns a non-zero OSStatus into a Go error with the system's
// human-readable message (when available).
func osErr(op string, st C.OSStatus) error {
	cmsg := C.kc_errmsg(st)
	if cmsg == nil {
		return fmt.Errorf("%s: OSStatus %d", op, int32(st))
	}
	defer C.free(unsafe.Pointer(cmsg))
	return fmt.Errorf("%s: %s (OSStatus %d)", op, C.GoString(cmsg), int32(st))
}
