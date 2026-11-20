//go:build darwin && cgo

// Package coreml runs a compiled CoreML program with an input x
// [1, s, h] and mask [1, 1, 1, s] and an output logits [1, s, k]: the
// part of internal/pii/guard that links CoreML, apart as a package with
// cgo can't have guard's Go assembly.
package coreml

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreML -framework Foundation
#include <stdlib.h>
#include <string.h>
#import <CoreML/CoreML.h>

static char *gmlErr(NSError *e, const char *what) {
	if (e == nil) return strdup(what);
	return strdup(e.localizedDescription.UTF8String ?: what);
}

// gmlCompile compiles the package at pkg into out (a .mlmodelc).
static char *gmlCompile(const char *pkg, const char *out) {
	@autoreleasepool {
		if (@available(macOS 13, *)) {
		} else {
			return strdup("CoreML programs need macOS 13");
		}
		NSError *err = nil;
		NSURL *c = [MLModel compileModelAtURL:[NSURL fileURLWithPath:@(pkg)] error:&err];
		if (c == nil) return gmlErr(err, "compiling failed");
		NSFileManager *fm = [NSFileManager defaultManager];
		NSURL *o = [NSURL fileURLWithPath:@(out)];
		[fm removeItemAtURL:o error:nil];
		if (![fm moveItemAtURL:c toURL:o error:&err]) {
			[fm removeItemAtURL:c error:nil];
			return gmlErr(err, "moving the compiled model failed");
		}
		return NULL;
	}
}

static void *gmlOpen(const char *path, char **errp) {
	@autoreleasepool {
		MLModelConfiguration *cfg = [MLModelConfiguration new];
		cfg.computeUnits = MLComputeUnitsAll;
		NSError *err = nil;
		MLModel *m = [MLModel modelWithContentsOfURL:[NSURL fileURLWithPath:@(path)] configuration:cfg error:&err];
		if (m == nil) {
			*errp = gmlErr(err, "loading failed");
			return NULL;
		}
		return (__bridge_retained void *)m;
	}
}

static void gmlClose(void *m) { CFRelease(m); }

static MLMultiArray *gmlArray(const float *src, NSArray<NSNumber *> *shape, NSArray<NSNumber *> *strides, size_t n, NSError **err) {
	float *b = malloc(n * sizeof(float));
	memcpy(b, src, n * sizeof(float));
	return [[MLMultiArray alloc] initWithDataPointer:b shape:shape dataType:MLMultiArrayDataTypeFloat32
		strides:strides deallocator:^(void *p) { free(p); } error:err];
}

// gmlRun reads x [1, s, h] and mask [1, 1, 1, s] into out [s, k].
static char *gmlRun(void *mp, const float *x, const float *mask, long s, long h, float *out, long k) {
	@autoreleasepool {
		MLModel *m = (__bridge MLModel *)mp;
		NSError *err = nil;
		MLMultiArray *xa = gmlArray(x, @[@1, @(s), @(h)], @[@(s * h), @(h), @1], s * h, &err);
		if (xa == nil) return gmlErr(err, "x");
		MLMultiArray *ma = gmlArray(mask, @[@1, @1, @1, @(s)], @[@(s), @(s), @(s), @1], s, &err);
		if (ma == nil) return gmlErr(err, "mask");
		MLDictionaryFeatureProvider *in = [[MLDictionaryFeatureProvider alloc] initWithDictionary:@{
			@"x": [MLFeatureValue featureValueWithMultiArray:xa],
			@"mask": [MLFeatureValue featureValueWithMultiArray:ma],
		} error:&err];
		if (in == nil) return gmlErr(err, "inputs");
		id<MLFeatureProvider> r = [m predictionFromFeatures:in error:&err];
		if (r == nil) return gmlErr(err, "prediction failed");
		MLMultiArray *lg = [r featureValueForName:@"logits"].multiArrayValue;
		if (lg == nil || lg.shape.count != 3 || lg.shape[1].longValue != s || lg.shape[2].longValue != k) return strdup("logits have another shape");
		long s1 = lg.strides[1].longValue, s2 = lg.strides[2].longValue;
		MLMultiArrayDataType dt = lg.dataType;
		if (dt != MLMultiArrayDataTypeFloat32 && dt != MLMultiArrayDataTypeFloat16) return strdup("logits are of another type");
		[lg getBytesWithHandler:^(const void *b, NSInteger size) {
			for (long t = 0; t < s; t++) {
				for (long c = 0; c < k; c++) {
					long i = t * s1 + c * s2;
					out[t * k + c] = dt == MLMultiArrayDataTypeFloat32 ? ((const float *)b)[i] : (float)((const __fp16 *)b)[i];
				}
			}
		}];
		return NULL;
	}
}
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"
)

// Available says CoreML is here.
const Available = true

// Compile compiles the package at pkg into out (a .mlmodelc).
func Compile(pkg, out string) error {
	cp, co := C.CString(pkg), C.CString(out)
	defer C.free(unsafe.Pointer(cp))
	defer C.free(unsafe.Pointer(co))
	if ce := C.gmlCompile(cp, co); ce != nil {
		defer C.free(unsafe.Pointer(ce))
		return errors.New(C.GoString(ce))
	}
	return nil
}

// Model is a compiled program, opened for the Neural Engine, the GPU or
// the CPU, whichever CoreML picks; it is let go with the last reference.
type Model struct {
	mu sync.Mutex
	m  unsafe.Pointer
}

// Open opens the compiled program at path.
func Open(path string) (*Model, error) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	var ce *C.char
	p := C.gmlOpen(cp, &ce)
	if p == nil {
		defer C.free(unsafe.Pointer(ce))
		return nil, errors.New(C.GoString(ce))
	}
	m := &Model{m: p}
	runtime.AddCleanup(m, func(p unsafe.Pointer) { C.gmlClose(p) }, p)
	return m, nil
}

// Run is the logits [s, k] for x [s, h] and mask [s].
func (m *Model) Run(x, mask []float32, s, h, k int) ([]float32, error) {
	if len(x) != s*h || len(mask) != s {
		return nil, errors.New("inputs of another size")
	}
	out := make([]float32, s*k)
	m.mu.Lock()
	ce := C.gmlRun(m.m, (*C.float)(&x[0]), (*C.float)(&mask[0]), C.long(s), C.long(h), (*C.float)(&out[0]), C.long(k))
	m.mu.Unlock()
	runtime.KeepAlive(m)
	if ce != nil {
		defer C.free(unsafe.Pointer(ce))
		return nil, errors.New(C.GoString(ce))
	}
	return out, nil
}
