/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package amqp

// #include <proton/codec.h>
import "C"

import (
	"fmt"
	"io"
	"reflect"
	"time"
	"unsafe"
)

// Error returned if Go data cannot be marshaled as an AMQP type.
type MarshalError struct {
	// The Go type.
	GoType reflect.Type
	s      string
}

func (e MarshalError) Error() string { return e.s }

func newMarshalError(v interface{}, s string) *MarshalError {
	t := reflect.TypeOf(v)
	return &MarshalError{GoType: t, s: fmt.Sprintf("cannot marshal %s: %s", t, s)}
}

func dataMarshalError(v interface{}, data *C.pn_data_t) error {
	if pe := PnError(C.pn_data_error(data)); pe != nil {
		return newMarshalError(v, pe.Error())
	}
	return nil
}

func recoverMarshal(err *error) {
	if r := recover(); r != nil {
		if merr, ok := r.(*MarshalError); ok {
			*err = merr
		} else {
			panic(r)
		}
	}
}

/*
Marshal encodes a Go value as AMQP data in buffer.
If buffer is nil, or is not large enough, a new buffer  is created.

Returns the buffer used for encoding with len() adjusted to the actual size of data.

Go types are encoded as follows

 +-------------------------------------+--------------------------------------------+
 |Go type                              |AMQP type                                   |
 +-------------------------------------+--------------------------------------------+
 |bool                                 |bool                                        |
 +-------------------------------------+--------------------------------------------+
 |int8, int16, int32, int64 (int)      |byte, short, int, long (int or long)        |
 +-------------------------------------+--------------------------------------------+
 |uint8, uint16, uint32, uint64 (uint) |ubyte, ushort, uint, ulong (uint or ulong)  |
 +-------------------------------------+--------------------------------------------+
 |float32, float64                     |float, double.                              |
 +-------------------------------------+--------------------------------------------+
 |string                               |string                                      |
 +-------------------------------------+--------------------------------------------+
 |[]byte, Binary                       |binary                                      |
 +-------------------------------------+--------------------------------------------+
 |Symbol                               |symbol                                      |
 +-------------------------------------+--------------------------------------------+
 |Char                                 |char                                        |
 +-------------------------------------+--------------------------------------------+
 |interface{}                          |the contained type                          |
 +-------------------------------------+--------------------------------------------+
 |nil                                  |null                                        |
 +-------------------------------------+--------------------------------------------+
 |map[K]T                              |map with K and T converted as above         |
 +-------------------------------------+--------------------------------------------+
 |Map                                  |map, may have mixed types for keys, values  |
 +-------------------------------------+--------------------------------------------+
 |List, []interface{}                  |list, may have mixed-type values            |
 +-------------------------------------+--------------------------------------------+
 |[]T, [N]T                            |array, T is mapped as per this table        |
 +-------------------------------------+--------------------------------------------+
 |Described                            |described type                              |
 +-------------------------------------+--------------------------------------------+
 |time.Time                            |timestamp                                   |
 +-------------------------------------+--------------------------------------------+
 |UUID                                 |uuid                                        |
 +-------------------------------------+--------------------------------------------+

The following Go types cannot be marshaled: uintptr, function, channel, struct, complex64/128

AMQP types not yet supported:
- decimal32/64/128,
*/

func Marshal(v interface{}, buffer []byte) (outbuf []byte, err error) {
	defer recoverMarshal(&err)
	data := C.pn_data(0)
	defer C.pn_data_free(data)
	marshal(v, data)
	encode := func(buf []byte) ([]byte, error) {
		n := int(C.pn_data_encode(data, cPtr(buf), cLen(buf)))
		switch {
		case n == int(C.PN_OVERFLOW):
			return buf, overflow
		case n < 0:
			return buf, dataMarshalError(v, data)
		default:
			return buf[:n], nil
		}
	}
	return encodeGrow(buffer, encode)
}

// Internal
func MarshalUnsafe(v interface{}, pn_data unsafe.Pointer) (err error) {
	defer recoverMarshal(&err)
	marshal(v, (*C.pn_data_t)(pn_data))
	return
}

const minEncode = 256

// overflow is returned when an encoding function can't fit data in the buffer.
var overflow = fmt.Errorf("buffer too small")

// encodeFn encodes into buffer[0:len(buffer)].
// Returns buffer with length adjusted for data encoded.
// If buffer too small, returns overflow as error.
type encodeFn func(buffer []byte) ([]byte, error)

// encodeGrow calls encode() into buffer, if it returns overflow grows the buffer.
// Returns the final buffer.
func encodeGrow(buffer []byte, encode encodeFn) ([]byte, error) {
	if buffer == nil || len(buffer) == 0 {
		buffer = make([]byte, minEncode)
	}
	var err error
	for buffer, err = encode(buffer); err == overflow; buffer, err = encode(buffer) {
		buffer = make([]byte, 2*len(buffer))
	}
	return buffer, err
}

const intIsLong bool = (unsafe.Sizeof(int(0)) == 8)

// Marshal v to data if data != nil
// Return the pn_type_t for v, even if data == nil
func marshal(i interface{}, data *C.pn_data_t) C.pn_type_t {
	if data != nil { // On exit, check for errors on the data object
		defer func() {
			if err := dataMarshalError(i, data); err != nil {
				panic(err)
			}
		}()
	}
	switch v := i.(type) {
	case nil:
		if data != nil {
			C.pn_data_put_null(data)
		}
		return C.PN_NULL
	case bool:
		if data != nil {
			C.pn_data_put_bool(data, C.bool(v))
		}
		return C.PN_BOOL
	case int8:
		if data != nil {
			C.pn_data_put_byte(data, C.int8_t(v))
		}
		return C.PN_BYTE
	case int16:
		if data != nil {
			C.pn_data_put_short(data, C.int16_t(v))
		}
		return C.PN_SHORT
	case int32:
		if data != nil {
			C.pn_data_put_int(data, C.int32_t(v))
		}
		return C.PN_INT
	case int64:
		if data != nil {
			C.pn_data_put_long(data, C.int64_t(v))
		}
		return C.PN_LONG
	case int:
		if intIsLong {
			C.pn_data_put_long(data, C.int64_t(v))
			return C.PN_LONG
		} else {
			C.pn_data_put_int(data, C.int32_t(v))
			return C.PN_INT
		}
	case uint8:
		if data != nil {
			C.pn_data_put_ubyte(data, C.uint8_t(v))
		}
		return C.PN_UBYTE
	case uint16:
		if data != nil {
			C.pn_data_put_ushort(data, C.uint16_t(v))
		}
		return C.PN_USHORT
	case uint32:
		if data != nil {
			C.pn_data_put_uint(data, C.uint32_t(v))
		}
		return C.PN_UINT
	case uint64:
		if data != nil {
			C.pn_data_put_ulong(data, C.uint64_t(v))
		}
		return C.PN_ULONG
	case uint:
		if intIsLong {
			C.pn_data_put_ulong(data, C.uint64_t(v))
			return C.PN_ULONG
		} else {
			C.pn_data_put_uint(data, C.uint32_t(v))
			return C.PN_UINT
		}
	case float32:
		if data != nil {
			C.pn_data_put_float(data, C.float(v))
		}
		return C.PN_FLOAT
	case float64:
		if data != nil {
			C.pn_data_put_double(data, C.double(v))
		}
		return C.PN_DOUBLE
	case string:
		if data != nil {
			C.pn_data_put_string(data, pnBytes([]byte(v)))
		}
		return C.PN_STRING

	case []byte:
		if data != nil {
			C.pn_data_put_binary(data, pnBytes(v))
		}
		return C.PN_BINARY

	case Binary:
		if data != nil {
			C.pn_data_put_binary(data, pnBytes([]byte(v)))
		}
		return C.PN_BINARY

	case Symbol:
		if data != nil {
			C.pn_data_put_symbol(data, pnBytes([]byte(v)))
		}
		return C.PN_SYMBOL

	case Described:
		C.pn_data_put_described(data)
		C.pn_data_enter(data)
		marshal(v.Descriptor, data)
		marshal(v.Value, data)
		C.pn_data_exit(data)
		return C.PN_DESCRIBED

	case AnnotationKey:
		return marshal(v.Get(), data)

	case time.Time:
		if data != nil {
			C.pn_data_put_timestamp(data, C.pn_timestamp_t(v.UnixNano()/1000))
		}
		return C.PN_TIMESTAMP

	case UUID:
		if data != nil {
			C.pn_data_put_uuid(data, *(*C.pn_uuid_t)(unsafe.Pointer(&v[0])))
		}
		return C.PN_UUID

	case Char:
		if data != nil {
			C.pn_data_put_char(data, (C.pn_char_t)(v))
		}
		return C.PN_CHAR

	default:
		// Look at more complex types by reflected structure

		switch reflect.TypeOf(i).Kind() {

		case reflect.Map:
			if data != nil {
				m := reflect.ValueOf(v)
				C.pn_data_put_map(data)
				C.pn_data_enter(data)
				defer C.pn_data_exit(data)
				for _, key := range m.MapKeys() {
					marshal(key.Interface(), data)
					marshal(m.MapIndex(key).Interface(), data)
				}
			}
			return C.PN_MAP

		case reflect.Slice, reflect.Array:
			// Note: Go array and slice are mapped the same way:
			// if element type is an interface, map to AMQP list (mixed type)
			// if element type is a non-interface type map to AMQP array (single type)
			s := reflect.ValueOf(v)
			var ret C.pn_type_t
			t := reflect.TypeOf(i).Elem()
			if t.Kind() == reflect.Interface {
				if data != nil {
					C.pn_data_put_list(data)
				}
				ret = C.PN_LIST
			} else {
				if data != nil {
					pnType := marshal(reflect.Zero(t).Interface(), nil)
					C.pn_data_put_array(data, false, pnType)
				}
				ret = C.PN_ARRAY
			}
			if data != nil {
				C.pn_data_enter(data)
				defer C.pn_data_exit(data)
				for j := 0; j < s.Len(); j++ {
					marshal(s.Index(j).Interface(), data)
				}
			}
			return ret

		default:
			panic(newMarshalError(v, "no conversion"))
		}
	}
}

func clearMarshal(v interface{}, data *C.pn_data_t) {
	C.pn_data_clear(data)
	marshal(v, data)
}

// Encoder encodes AMQP values to an io.Writer
type Encoder struct {
	writer io.Writer
	buffer []byte
}

// New encoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w, make([]byte, minEncode)}
}

func (e *Encoder) Encode(v interface{}) (err error) {
	e.buffer, err = Marshal(v, e.buffer)
	if err == nil {
		_, err = e.writer.Write(e.buffer)
	}
	return err
}
