package mg

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var _ Dispatcher = (&Store{}).Dispatch

type Dispatcher func(Action)

type Listener func(*State)

type storeReducers struct {
	before ReducerList
	use    ReducerList
	after  ReducerList
}

func (sr storeReducers) Reduce(mx *Ctx) *State {
	mx = sr.before.ReduceCtx(mx)
	mx = sr.use.ReduceCtx(mx)
	mx = sr.after.ReduceCtx(mx)
	return mx.State
}

func (sr storeReducers) Copy(updaters ...func(*storeReducers)) storeReducers {
	for _, f := range updaters {
		f(&sr)
	}
	return sr
}

type Store struct {
	KVMap

	mu        sync.Mutex
	readyCh   chan struct{}
	state     *State
	listeners []*struct{ Listener }
	listener  Listener
	reducers  struct {
		sync.Mutex
		storeReducers
	}
	cfg   EditorConfig
	ag    *Agent
	tasks *taskTracker
	cache struct {
		sync.RWMutex
		vName string
		vHash string
	}
}

func (sto *Store) ready() {
	close(sto.readyCh)
}

func (sto *Store) Dispatch(act Action) {
	go func() {
		<-sto.readyCh
		sto.dispatch(act)
	}()
}

func (sto *Store) dispatch(act Action) {
	sto.mu.Lock()
	defer sto.mu.Unlock()

	mx, done := newCtx(sto.ag, sto.prepState(sto.state), act, sto)
	defer close(done)
	st := sto.reducers.Reduce(mx)
	sto.updateState(st, true)
}

func (sto *Store) syncRq(ag *Agent, rq *agentReq) {
	sto.mu.Lock()
	defer sto.mu.Unlock()

	rs := agentRes{Cookie: rq.Cookie, State: sto.state}
	for _, ra := range rq.Actions {
		st, err := sto.syncRqAct(ag, rq.Props, ra)
		if st != nil {
			sto.state = st // normally sto.updateState would do this
			rs.State = st
		}
		if err != nil {
			rs.Error = err.Error()
		}
	}
	rs.State = sto.updateState(rs.State, false)
	ag.send(rs)
}

func (sto *Store) syncRqAct(ag *Agent, props clientProps, ra agentReqAction) (*State, error) {
	act, err := ag.createAction(ra, ag.handle)
	if err != nil {
		return nil, err
	}
	mx, done := newCtx(sto.ag, sto.state, act, sto)
	defer close(done)

	mx = mx.Copy(func(mx *Ctx) {
		st := sto.prepState(mx.State)
		st.Editor = props.Editor.EditorProps
		if props.Env != nil {
			st.Env = props.Env
		}

		if props.View != nil {
			st.View = props.View.Copy(func(v *View) {
				sto.initCache(v)
				v.initSrcPos()
			})
		}

		osGopath := os.Getenv("GOPATH")
		fn := st.View.Filename()
		for _, dir := range strings.Split(osGopath, string(filepath.ListSeparator)) {
			if IsParentDir(dir, fn) {
				st.Env = st.Env.Add("GOPATH", osGopath)
				break
			}
		}
		mx.State = st
	})

	// sto.initCache(mx.View)
	// mx.State = sto.prepState(mx.State)
	return sto.reducers.Reduce(mx), nil
}

func (sto *Store) updateState(st *State, callListener bool) *State {
	if callListener && sto.listener != nil {
		sto.listener(st)
	}
	for _, p := range sto.listeners {
		p.Listener(st)
	}
	sto.state = st
	return st
}

func (sto *Store) State() *State {
	sto.mu.Lock()
	defer sto.mu.Unlock()

	return sto.state
}

func (sto *Store) prepState(st *State) *State {
	st = st.new()
	if sto.cfg != nil {
		st.Config = sto.cfg
	}
	return st
}

func newStore(ag *Agent, l Listener) *Store {
	sto := &Store{
		readyCh:  make(chan struct{}),
		listener: l,
		state:    newState(ag.Store),
		ag:       ag,
	}
	sto.tasks = newTaskTracker(sto.Dispatch)
	sto.After(sto.tasks)
	return sto
}

func (sto *Store) Subscribe(l Listener) (unsubscribe func()) {
	sto.mu.Lock()
	defer sto.mu.Unlock()

	p := &struct{ Listener }{l}
	sto.listeners = append(sto.listeners[:len(sto.listeners):len(sto.listeners)], p)

	return func() {
		sto.mu.Lock()
		defer sto.mu.Unlock()

		listeners := make([]*struct{ Listener }, 0, len(sto.listeners)-1)
		for _, q := range sto.listeners {
			if p != q {
				listeners = append(listeners, q)
			}
		}
		sto.listeners = listeners
	}
}

func (sto *Store) updateReducers(updaters ...func(*storeReducers)) *Store {
	sto.reducers.Lock()
	defer sto.reducers.Unlock()

	sto.reducers.storeReducers = sto.reducers.Copy(updaters...)
	return sto
}

func (sto *Store) Before(reducers ...Reducer) *Store {
	return sto.updateReducers(func(sr *storeReducers) {
		sr.before = sr.before.Add(reducers...)
	})
}

func (sto *Store) Use(reducers ...Reducer) *Store {
	return sto.updateReducers(func(sr *storeReducers) {
		sr.use = sr.use.Add(reducers...)
	})
}

func (sto *Store) After(reducers ...Reducer) *Store {
	return sto.updateReducers(func(sr *storeReducers) {
		sr.after = sr.after.Add(reducers...)
	})
}

func (sto *Store) EditorConfig(cfg EditorConfig) *Store {
	sto.mu.Lock()
	defer sto.mu.Unlock()

	sto.cfg = cfg
	return sto
}

func (sto *Store) Begin(t Task) *TaskTicket {
	return sto.tasks.Begin(t)
}

func (sto *Store) initCache(v *View) {
	cc := &sto.cache
	cc.Lock()
	defer cc.Unlock()

	if cc.vHash == v.Hash && cc.vName == v.Name {
		return
	}

	sto.KVMap.Clear()
	cc.vHash = v.Hash
	cc.vName = v.Name
}
