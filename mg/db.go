package mg

import (
	"sync"
)

var (
	_ KVStore = (KVStores)(nil)
	_ KVStore = (*Store)(nil)
	_ KVStore = (*KVMap)(nil)
)

// KVStore represents a generic key value store.
//
// All operations are safe for concurrent access.
//
// The main implementation in this and related packages is Store.
type KVStore interface {
	// Put stores the value in the store with identifier key
	// NOTE: use Del instead of storing nil
	Put(key, value interface{})

	// Get returns the value stored using identifier key
	// NOTE: if the value doesn't exist, nil is returned
	Get(key interface{}) interface{}

	// Del removes the value identified by key from the store
	Del(key interface{})
}

// KVStores implements a KVStore that duplicates its operations on a list of k/v stores
//
// NOTE: All operations are no-ops for nil KVStores
type KVStores []KVStore

// Put calls .Put on each of k/v stores in the list
func (kvl KVStores) Put(k, v interface{}) {
	for _, kvs := range kvl {
		if kvs == nil {
			continue
		}
		kvs.Put(k, v)
	}
}

// Get returns the first value identified by k found in the list of k/v stores
func (kvl KVStores) Get(k interface{}) interface{} {
	for _, kvs := range kvl {
		if kvs == nil {
			continue
		}
		if v := kvs.Get(k); v != nil {
			return v
		}
	}
	return nil
}

// Del removed the value identified by k from all k/v stores in the list
func (kvl KVStores) Del(k interface{}) {
	for _, kvs := range kvl {
		if kvs == nil {
			continue
		}
		kvs.Del(k)
	}
}

// KVMap implements a KVStore using a map.
// The zero-value is safe for use with all operations.
//
// NOTE: All operations are no-ops on a nil KVMap
type KVMap struct {
	vals map[interface{}]interface{}
	mu   sync.RWMutex
}

// Put implements KVStore.Put
func (m *KVMap) Put(k interface{}, v interface{}) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.vals == nil {
		m.vals = map[interface{}]interface{}{}
	}

	m.vals[k] = v
}

// Get implements KVStore.Get
func (m *KVMap) Get(k interface{}) interface{} {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.vals[k]
}

func (m *KVMap) GetDefault(k interface{}, def func() interface{}) interface{} {
	if m == nil {
		if def == nil {
			return nil
		}
		return def()
	}

	m.mu.RLock()
	v := m.vals[k]
	m.mu.RUnlock()

	if v != nil || def == nil {
		return v
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if v := m.vals[k]; v != nil {
		return v
	}

	if m.vals == nil {
		m.vals = map[interface{}]interface{}{}
	}
	v = def()
	m.vals[k] = v
	return v
}

// Del implements KVStore.Del
func (m *KVMap) Del(k interface{}) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.vals, k)
}

// Clear removes all values from the store
func (m *KVMap) Clear() {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.vals = map[interface{}]interface{}{}
}

// Values returns a copy of all values stored
func (m *KVMap) Values() map[interface{}]interface{} {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	vals := make(map[interface{}]interface{}, len(m.vals))
	for k, v := range m.vals {
		vals[k] = v
	}
	return vals
}
