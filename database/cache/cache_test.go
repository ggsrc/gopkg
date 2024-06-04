package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/s2"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/ggsrc/gopkg/database/cache"
)

const (
	longTxt1 = `// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v. If v is nil or not a pointer,
// Unmarshal returns an InvalidUnmarshalError.
//
// Unmarshal uses the inverse of the encodings that
// Marshal uses, allocating maps, slices, and pointers as necessary,
// with the following additional rules:`
	longTxt2 = `
// To unmarshal JSON into a pointer, Unmarshal first handles the case of
// the JSON being the JSON literal null. In that case, Unmarshal sets
// the pointer to nil. Otherwise, Unmarshal unmarshals the JSON into
// the value pointed at by the pointer. If the pointer is nil, Unmarshal
// allocates a new value for it to point to.`
	longTxt3 = `
// To unmarshal JSON into a value implementing the Unmarshaler interface,
// Unmarshal calls that value's UnmarshalJSON method, including
// when the input is a JSON null.
// Otherwise, if the value implements encoding.TextUnmarshaler
// and the input is a JSON quoted string, Unmarshal calls that value's
// UnmarshalText method with the unquoted form of the string.
//
// To unmarshal JSON into a struct, Unmarshal matches incoming object
// keys to the keys used by Marshal (either the struct field name or its tag),
// preferring an exact match but also accepting a case-insensitive match. By
// default, object keys which don't have a corresponding struct field are
// ignored (see Decoder.DisallowUnknownFields for an alternative).`
	longTxt4 = `
// To unmarshal JSON into an interface value,
// Unmarshal stores one of these in the interface value:
//
//	bool, for JSON booleans
//	float64, for JSON numbers
//	string, for JSON strings
//	[]interface{}, for JSON arrays
//	map[string]interface{}, for JSON objects
//	nil for JSON null
//
// To unmarshal a JSON array into a slice, Unmarshal resets the slice length
// to zero and then appends each element to the slice.
// As a special case, to unmarshal an empty JSON array into a slice,
// Unmarshal replaces the slice with a new empty slice.
//
// To unmarshal a JSON array into a Go array, Unmarshal decodes
// JSON array elements into corresponding Go array elements.
// If the Go array is smaller than the JSON array,
// the additional JSON array elements are discarded.
// If the JSON array is smaller than the Go array,
// the additional Go array elements are set to zero values.`
)

func getLocalRedisCli() redis.UniversalClient {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	client.FlushAll(context.Background())
	return client
}

func TestGet(t *testing.T) {
	c, err := cache.NewCache(getLocalRedisCli(), time.Millisecond*100, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test_key1"
	value := "test_value1"
	counter := 0
	testFunc := func() (interface{}, error) {
		counter++
		return value, nil
	}

	var v string
	err = c.Get(ctx, key, &v, time.Minute, testFunc)
	if err != nil {
		t.Fatal(err)
	}

	if counter != 1 {
		t.Fatal("counter should be 1")
	}
	if v != value {
		t.Fatal("value should be test_value")
	}

	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(ctx)
		var v string
		err = c.Get(ctx, key, &v, time.Minute, testFunc)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if v != value {
			t.Fatal("value should be test_value")
		}
	}
	if counter != 1 {
		t.Fatal("counter should be 1")
	}

	err = c.Invalidate(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetObject(t *testing.T) {
	type dt struct {
		Name string
		Age  int
	}

	c, err := cache.NewCache(getLocalRedisCli(), time.Millisecond*100, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test_key1"

	var rst dt

	// get from func
	err = c.Get(ctx, key, &rst, time.Minute, func() (interface{}, error) {
		return dt{
			Name: "test_name",
			Age:  10,
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if rst.Name != "test_name" || rst.Age != 10 {
		t.Fatal("value should be test_value")
	}

	// get from cache
	err = c.Get(ctx, key, &rst, time.Minute, func() (interface{}, error) {
		panic("should not be called")
	})
	if err != nil {
		t.Fatal(err)
	}

	if rst.Name != "test_name" || rst.Age != 10 {
		t.Fatal("value should be test_value")
	}
}

func TestCustomExpireGet(t *testing.T) {
	c, err := cache.NewCache(getLocalRedisCli(), time.Second*3, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test_keyx"
	value := "test_valuex"
	counter := 0
	var testFunc cache.PassThroughExpireFunc = func() (interface{}, time.Duration, error) {
		// Expire fast
		expire := 50 * time.Millisecond
		counter++
		return value, expire, nil
	}

	var target string
	err = c.GetWithExpire(ctx, key, &target, testFunc)
	if err != nil {
		t.Fatal(err)
	}
	if counter != 1 {
		t.Fatal("counter should be 1")
	}
	if target != value {
		t.Fatal("value should be test_value")
	}

	err = c.GetWithExpire(ctx, key, &target, testFunc)
	if err != nil {
		t.Fatal(err)
	}
	if counter != 1 {
		t.Fatal("counter should be 1")
	}
	if target != value {
		t.Fatal("value should be test_value")
	}

	// Should expire after 1 sec
	time.Sleep(time.Second)

	// req 1
	ctx, cancel := context.WithCancel(context.Background())
	err = c.GetWithExpire(ctx, key, &target, testFunc)
	cancel() // request done
	if err != nil {
		t.Fatal(err)
	}
	if target != value {
		t.Fatal("value should be test_value")
	}
	if counter != 2 {
		t.Fatal("counter should be 2", counter)
	}

	// req 2
	ctx, cancel = context.WithCancel(context.Background())
	err = c.GetWithExpire(ctx, key, &target, testFunc)
	cancel() // request done
	if err != nil {
		t.Fatal(err)
	}
	if target != value {
		t.Fatal("value should be test_value")
	}

	if counter != 2 {
		t.Fatal("counter should be 2", counter)
	}

	err = c.Invalidate(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInvalidate(t *testing.T) {
	c, err := cache.NewCache(getLocalRedisCli(), time.Second*3, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test_keyx"
	value := "test_valuex"
	counter := 0
	testFunc := func() (interface{}, error) {
		counter++
		return value, nil
	}

	var target string
	err = c.Get(ctx, key, &target, time.Minute, testFunc)
	if err != nil {
		t.Fatal(err)
	}

	if counter != 1 {
		t.Fatal("counter should be 1")
	}
	if target != value {
		t.Fatal("value should be test_value")
	}

	err = c.Invalidate(ctx, key)
	if err != nil {
		t.Fatal(err)
	}

	// req 1
	ctx, cancel := context.WithCancel(context.Background())
	err = c.Get(ctx, key, &target, time.Minute, testFunc)
	cancel() // request done
	if err != nil {
		t.Fatal(err)
	}
	if target != value {
		t.Fatal("value should be test_value")
	}

	// req 2
	ctx, cancel = context.WithCancel(context.Background())
	err = c.Get(ctx, key, &target, time.Minute, testFunc)
	cancel() // request done
	if err != nil {
		t.Fatal(err)
	}
	if target != value {
		t.Fatal("value should be test_value")
	}

	if counter != 2 {
		t.Fatal("counter should be 2", counter)
	}

	err = c.Invalidate(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentReadWait(t *testing.T) {
	c, err := cache.NewCache(getLocalRedisCli(), time.Millisecond*100, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test_key2"
	value := "test_value2"
	counter := 0
	testFunc := func() (interface{}, error) {
		counter++
		return value, nil
	}
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(testFunc func() (interface{}, error)) {
			defer wg.Done()
			var target string
			err := c.Get(ctx, key, &target, time.Minute, testFunc)
			if err != nil {
				t.Error(err)
			}
			if target != value {
				t.Error("value should be test_value")
			}
		}(testFunc)
	}
	wg.Wait()
	if counter != 1 {
		t.Fatal("counter should be 1", counter)
	}

}

func TestGetTimeout(t *testing.T) {
	c, err := cache.NewCache(getLocalRedisCli(), time.Second*10, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "test_key" + time.Now().String()
	value := "test_value3"
	testFunc := func() (interface{}, error) {
		time.Sleep(10 * time.Second)
		return value, nil
	}
	ch := make(chan bool)

	go func() {
		ch <- true
		var target string
		err = c.Get(ctx, key, target, time.Minute, testFunc)
		if err != nil {
			t.Error("Shouldn't err")
		}
	}()

	// wait routine to start and get lock
	<-ch
	time.Sleep(time.Millisecond * 100)
	var target string
	err = c.Get(ctx, key, &target, time.Minute, testFunc)
	if err == nil {
		t.Fatal("Should err")
	}
}

func TestMarshal(t *testing.T) {
	type dt struct {
		Name string
		Age  int
	}

	v0 := &dt{
		Name: "1",
		Age:  1,
	}

	// prt
	bs, err := _marshal(v0)
	if err != nil {
		t.Fatal(err)
	}
	var v0r dt
	err = _unmarshal(bs, &v0r)
	if err != nil {
		t.Fatal(err)
	}
	if v0r.Name != v0.Name || v0r.Age != v0.Age {
		t.Fatal("Not equal")
	}
	// struct
	v1 := dt{
		Name: "2",
		Age:  2,
	}
	bs, err = _marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	var v1r dt
	err = _unmarshal(bs, &v1r)
	if err != nil {
		t.Fatal(err)
	}
	if v1r.Name != v1.Name || v1r.Age != v1.Age {
		t.Fatal("Not equal")
	}

	// nil
	var v2 *dt
	bs, err = _marshal(v2)
	if err != nil {
		t.Fatal(err)
	}
	var v2r dt
	err = _unmarshal(bs, &v2r)
	if err != nil {
		t.Fatal(err)
	}
	if v2r.Name != "" || v2r.Age != 0 {
		t.Fatal("Not equal")
	}

	// int
	v3 := 1
	bs, err = _marshal(v3)
	if err != nil {
		t.Fatal(err)
	}
	var v3r int
	err = _unmarshal(bs, &v3r)
	if err != nil {
		t.Fatal(err)
	}
	if v3r != 1 {
		t.Fatal("Not equal")
	}
	// []int
	v4 := []int{1, 2, 3}
	bs, err = _marshal(v4)
	if err != nil {
		t.Fatal(err)
	}
	var v4r []int
	err = _unmarshal(bs, &v4r)
	if err != nil {
		t.Fatal(err)
	}
	if len(v4r) != 3 || v4r[0] != 1 || v4r[1] != 2 || v4r[2] != 3 {
		t.Fatal("Not equal")
	}
	// map
	v5 := map[string]int{
		"1": 1,
		"2": 2,
	}
	bs, err = _marshal(v5)
	if err != nil {
		t.Fatal(err)
	}
	var v5r map[string]int
	err = _unmarshal(bs, &v5r)
	if err != nil {
		t.Fatal(err)
	}
	if len(v5r) != 2 || v5r["1"] != 1 || v5r["2"] != 2 {
		t.Fatal("Not equal")
	}
}

const (
	compressionThreshold = 64
	timeLen              = 4
)

const (
	noCompression = 0x0
	s2Compression = 0x1
)

func _marshal(value interface{}) ([]byte, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	}

	b, err := msgpack.Marshal(value)
	if err != nil {
		return nil, err
	}

	return compress(b), nil
}

func _marshalWithoutCompress(value interface{}) ([]byte, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	}

	b, err := msgpack.Marshal(value)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func compress(data []byte) []byte {
	if len(data) < compressionThreshold {
		n := len(data) + 1
		b := make([]byte, n, n+timeLen)
		copy(b, data)
		b[len(b)-1] = noCompression
		return b
	}

	n := s2.MaxEncodedLen(len(data)) + 1
	b := make([]byte, n, n+timeLen)
	b = s2.Encode(b, data)
	b = append(b, s2Compression)
	return b
}

func _unmarshal(b []byte, value interface{}) error {
	if len(b) == 0 {
		return nil
	}

	switch value := value.(type) {
	case nil:
		return nil
	case *[]byte:
		clone := make([]byte, len(b))
		copy(clone, b)
		*value = clone
		return nil
	case *string:
		*value = string(b)
		return nil
	}

	switch c := b[len(b)-1]; c {
	case noCompression:
		b = b[:len(b)-1]
	case s2Compression:
		b = b[:len(b)-1]

		var err error
		b, err = s2.Decode(nil, b)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown compression method: %x", c)
	}

	return msgpack.Unmarshal(b, value)
}

func _unmarshalWithoutCompress(b []byte, value interface{}) error {
	if len(b) == 0 {
		return nil
	}

	switch c := b[len(b)-1]; c {
	case noCompression:
		b = b[:len(b)-1]
	case s2Compression:
		b = b[:len(b)-1]

		var err error
		b, err = s2.Decode(nil, b)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown compression method: %x", c)
	}

	return msgpack.Unmarshal(b, value)
}

func BenchmarkMarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bytes, err := _marshal(i)
		if err != nil {
			b.Fatal(err)
		}
		var rst int
		err = _unmarshal(bytes, &rst)
		if err != nil {
			b.Fatal(err)
		}
		if rst != i {
			b.Fatal("Not equal")
		}
	}
}

func BenchmarkMarshalObj(b *testing.B) {
	/*
		cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
		BenchmarkMarshalObj
		BenchmarkMarshalObj-12    	  125700	      8039 ns/op
	*/
	type simpleStruct struct {
		Name     string
		Age      int
		LongTxt1 string
		LongTxt2 string
		LongTxt3 string
		LongTxt4 string
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bytes, _ := _marshal(&simpleStruct{
			Name:     fmt.Sprintf("name%d", i),
			Age:      i,
			LongTxt1: longTxt1,
			LongTxt2: longTxt2,
			LongTxt3: longTxt3,
			LongTxt4: longTxt4,
		})
		var rst simpleStruct
		_ = _unmarshal(bytes, &rst)
	}
}

func BenchmarkMarshalObjWithoutCompress(b *testing.B) {
	/*
		cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
		BenchmarkMarshalObjWithoutCompress
		BenchmarkMarshalObjWithoutCompress-12    	  744849	      1552 ns/op
	*/
	type simpleStruct struct {
		Name     string
		Age      int
		LongTxt1 string
		LongTxt2 string
		LongTxt3 string
		LongTxt4 string
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bytes, _ := _marshalWithoutCompress(&simpleStruct{
			Name:     fmt.Sprintf("name%d", i),
			Age:      i,
			LongTxt1: longTxt1,
			LongTxt2: longTxt2,
			LongTxt3: longTxt3,
			LongTxt4: longTxt4,
		})
		var rst simpleStruct
		_ = _unmarshalWithoutCompress(bytes, &rst)
	}
}
