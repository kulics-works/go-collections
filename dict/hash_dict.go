package dict

import (
	"hash/fnv"

	. "github.com/kulics/gollection"
	. "github.com/kulics/gollection/math"
	. "github.com/kulics/gollection/tuple"
	. "github.com/kulics/gollection/union"
)

const defaultElementsSize = 10

func NumberHasher[T Number](t T) int {
	return int(t)
}

func StringHasher[T ~string](t T) int {
	var h = fnv.New32a()
	h.Write([]byte(t))
	return int(h.Sum32())
}

func HashDictOf[K comparable, V any](hasher func(data K) int, elements ...Pair[K, V]) HashDict[K, V] {
	var size = len(elements)
	var dict = MakeHashDict[K, V](hasher, size)
	for _, v := range elements {
		dict.Put(v.First, v.Second)
	}
	return dict
}

func NumberDictOf[K Number, V any](elements ...Pair[K, V]) HashDict[K, V] {
	return HashDictOf(NumberHasher[K], elements...)
}

func StringDictOf[K ~string, V any](elements ...Pair[K, V]) HashDict[K, V] {
	return HashDictOf(StringHasher[K], elements...)
}

func MakeHashDict[K comparable, V any](hasher func(data K) int, capacity int) HashDict[K, V] {
	var size = capacity
	var buckets = make([]int, bucketsSizeFor(size))
	for i := 0; i < len(buckets); i++ {
		buckets[i] = -1
	}
	if size < defaultElementsSize {
		size = defaultElementsSize
	}
	var inner = &hashMap[K, V]{
		buckets:    buckets,
		entries:    make([]entry[K, V], size),
		hasher:     hasher,
		loadFactor: 1,
	}
	return HashDict[K, V]{inner}
}

func MakeNumberDict[K Number, V any](capacity int) HashDict[K, V] {
	return MakeHashDict[K, V](NumberHasher[K], capacity)
}

func MakeStringDict[K ~string, V any](capacity int) HashDict[K, V] {
	return MakeHashDict[K, V](StringHasher[K], capacity)
}

func HashDictFrom[K comparable, V any, I Collection[Pair[K, V]]](hasher func(data K) int, collection I) HashDict[K, V] {
	var size = collection.Size()
	var dict = MakeHashDict[K, V](hasher, size)
	ForEach(func(t Pair[K, V]) {
		dict.Put(t.First, t.Second)
	}, collection)
	return dict
}

func NumberDictFrom[K Number, V any, I Collection[Pair[K, V]]](collection I) HashDict[K, V] {
	return HashDictFrom[K, V](NumberHasher[K], collection)
}

func StringDictFrom[K ~string, V any, I Collection[Pair[K, V]]](collection I) HashDict[K, V] {
	return HashDictFrom[K, V](StringHasher[K], collection)
}

func bucketsSizeFor(size int) int {
	var bucketsSize = 16
	for bucketsSize < size {
		bucketsSize = bucketsSize * 2
	}
	return bucketsSize
}

type HashDict[K comparable, V any] struct {
	inner *hashMap[K, V]
}

type hashMap[K comparable, V any] struct {
	buckets     []int
	entries     []entry[K, V]
	appendCount int
	freeCount   int
	freeSize    int
	hasher      func(data K) int
	loadFactor  float64
}

type entry[K any, V any] struct {
	hash  int
	key   K
	value V
	next  int
	alive bool
}

func (a HashDict[K, V]) Get(key K) V {
	if v, ok := a.TryGet(key).Get(); ok {
		return v
	}
	panic(OutOfBounds)
}

func (a HashDict[K, V]) Put(key K, value V) Option[V] {
	var hash = a.inner.hasher(key)
	var index = a.index(hash)
	for i := a.inner.buckets[index]; i >= 0; i = a.inner.entries[i].next {
		var item = a.inner.entries[i]
		if item.hash == hash && item.key == key {
			var newItem = entry[K, V]{
				hash:  item.hash,
				key:   item.key,
				value: value,
				next:  item.next,
				alive: item.alive,
			}
			a.inner.entries[i] = newItem
			return Some(item.value)
		}
	}
	var bucket int
	if a.inner.freeSize > 0 {
		bucket = a.inner.freeCount
		a.inner.freeCount = a.inner.entries[a.inner.freeCount].next
		a.inner.freeSize--
	} else {
		a.grow(a.Size() + 1)
		bucket = a.inner.appendCount
		a.inner.appendCount++
	}
	var newItem = entry[K, V]{
		hash:  hash,
		key:   key,
		value: value,
		next:  a.inner.buckets[index],
		alive: true,
	}
	a.inner.entries[bucket] = newItem
	a.inner.buckets[index] = bucket
	return None[V]()
}

func (a HashDict[K, V]) PutAll(elements Collection[Pair[K, V]]) {
	var iter = elements.Iter()
	for item, ok := iter.Next().Get(); ok; item, ok = iter.Next().Get() {
		var k, v = item.Get()
		a.Put(k, v)
	}
}

func (a HashDict[K, V]) GetAndPut(key K, set func(oldValue Option[V]) V) Pair[V, Option[V]] {
	var hash = a.inner.hasher(key)
	var index = a.index(hash)
	for i := a.inner.buckets[index]; i >= 0; i = a.inner.entries[i].next {
		var item = a.inner.entries[i]
		if item.hash == hash && item.key == key {
			var newValue = set(Some(item.value))
			var newItem = entry[K, V]{
				hash:  item.hash,
				key:   item.key,
				value: newValue,
				next:  item.next,
				alive: item.alive,
			}
			a.inner.entries[i] = newItem
			return PairOf(newValue, Some(item.value))
		}
	}
	var bucket int
	if a.inner.freeSize > 0 {
		bucket = a.inner.freeCount
		a.inner.freeCount = a.inner.entries[a.inner.freeCount].next
		a.inner.freeSize--
	} else {
		a.grow(a.Size() + 1)
		bucket = a.inner.appendCount
		a.inner.appendCount++
	}
	var newValue = set(None[V]())
	var newItem = entry[K, V]{
		hash:  hash,
		key:   key,
		value: newValue,
		next:  a.inner.buckets[index],
		alive: true,
	}
	a.inner.entries[bucket] = newItem
	a.inner.buckets[index] = bucket
	return PairOf(newValue, None[V]())
}

func (a HashDict[K, V]) TryGet(key K) Option[V] {
	var hash = a.inner.hasher(key)
	var index = a.index(hash)
	for i := a.inner.buckets[index]; i >= 0; i = a.inner.entries[i].next {
		var item = a.inner.entries[i]
		if item.hash == hash && item.key == key {
			return Some(item.value)
		}
	}
	return None[V]()
}

func (a HashDict[K, V]) Remove(key K) Option[V] {
	var hash = a.inner.hasher(key)
	var index = a.index(hash)
	var last = -1
	for i := a.inner.buckets[index]; i >= 0; i = a.inner.entries[i].next {
		var item = a.inner.entries[i]
		if item.hash == hash && item.key == key {
			if last < 0 {
				a.inner.buckets[index] = a.inner.entries[i].next
			} else {
				var item = a.inner.entries[last]
				item.next = a.inner.entries[i].next
				a.inner.entries[last] = item
			}
			var nilK K
			var nilV V
			var empty = entry[K, V]{
				next:  a.inner.freeCount,
				key:   nilK,
				value: nilV,
			}
			a.inner.entries[i] = empty
			a.inner.freeCount = i
			a.inner.freeCount++
			return Some(item.value)
		}
	}
	return None[V]()
}

func (a HashDict[K, V]) Contains(key K) bool {
	return a.TryGet(key).IsSome()
}

func (a HashDict[K, V]) Size() int {
	return a.inner.appendCount - a.inner.freeSize + 1
}

func (a HashDict[K, V]) IsEmpty() bool {
	return a.Size() == 0
}

func (a HashDict[K, V]) Clear() {
	for i := 0; i < len(a.inner.buckets); i++ {
		a.inner.buckets[i] = -1
	}
	for i := 0; i < len(a.inner.entries); i++ {
		a.inner.entries[i] = entry[K, V]{}
	}
}

func (a HashDict[K, V]) Iter() Iterator[Pair[K, V]] {
	return &hashMapIterator[K, V]{-1, a}
}

func (a HashDict[K, V]) ToSlice() []Pair[K, V] {
	var arr = make([]Pair[K, V], a.Size())
	ForEach(func(t Pair[K, V]) {
		arr = append(arr, t)
	}, a)
	return arr
}

func (a HashDict[K, V]) grow(newSize int) {
	var entriesSize = len(a.inner.entries)
	var bucketsSize = len(a.inner.buckets)
	if float64(newSize/bucketsSize) > a.inner.loadFactor {
		var newBucketsSize = bucketsSize * 2
		var newBuckets = make([]int, newBucketsSize)
		for i := 0; i < len(newBuckets); i++ {
			newBuckets[i] = -1
		}
		for i, v := range a.inner.entries {
			if v.alive {
				var bucket = v.hash % newBucketsSize
				v.next = newBuckets[bucket]
				a.inner.entries[i] = v
				newBuckets[bucket] = i
			}
		}
		a.inner.buckets = newBuckets
	}
	if newSize > entriesSize {
		var newEntries = make([]entry[K, V], entriesSize+(entriesSize<<1))
		copy(newEntries, a.inner.entries)
		a.inner.entries = newEntries
	}
}

func (a HashDict[K, V]) index(hash int) int {
	return hash % len(a.inner.buckets)
}

type hashMapIterator[K comparable, V any] struct {
	index  int
	source HashDict[K, V]
}

func (a *hashMapIterator[K, V]) Next() Option[Pair[K, V]] {
	for a.index < len(a.source.inner.entries)-1 {
		a.index++
		var item = a.source.inner.entries[a.index]
		if item.alive {
			return Some(PairOf(item.key, item.value))
		}
	}
	return None[Pair[K, V]]()
}

func (a *hashMapIterator[K, V]) Iter() Iterator[Pair[K, V]] {
	return a
}