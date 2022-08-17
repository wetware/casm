package routing

import (
	"time"

	"github.com/wetware/casm/pkg/stm"
	"go.uber.org/atomic"
)

type Table struct {
	clock   *atomic.Time
	records stm.TableRef
	sched   stm.Scheduler
}

func New(t0 time.Time) Table {
	var (
		f     stm.Factory
		clock = atomic.NewTime(t0)
	)

	records := f.Register("record", schema(clock))
	sched, _ := f.NewScheduler() // no err since f is freshly instantiated

	return Table{
		clock:   clock,
		records: records,
		sched:   sched,
	}
}

func (table Table) NewQuery() Query {
	return &query{
		records: table.records,
		tx:      table.sched.Txn(false),
	}
}

// Advance the state of the routing table to the current time.
// Expired entries will be evicted from the table.
func (table Table) Advance(t time.Time) {
	if old := table.clock.Load(); t.After(old) {
		defer table.clock.Store(t)

		// Most ticks will not have expired entries, so avoid locking.
		if rx := table.sched.Txn(false); table.expiredRecords(rx, t) {
			wx := table.sched.Txn(true)
			defer wx.Commit()

			table.dropExpired(wx, t)
		}
	}
}

func (table Table) expiredRecords(tx stm.Txn, t time.Time) bool {
	it, _ := tx.ReverseLowerBound(table.records, "ttl", t)
	return it != nil && it.Next() != nil
}

func (table Table) dropExpired(wx stm.Txn, t time.Time) {
	it, _ := wx.ReverseLowerBound(table.records, "ttl", t)
	for r := it.Next(); r != nil; r = it.Next() {
		_ = wx.Delete(table.records, r)
	}
}

// Upsert inserts a record in the routing table, updating it
// if it already exists.  Returns true if rec is stale.
func (table Table) Upsert(rec Record) bool {
	// Some records are stale, so avoid locking until we're
	// sure to write.
	if rx := table.sched.Txn(false); table.valid(rx, rec) {
		wx := table.sched.Txn(true)
		defer wx.Commit()

		table.upsert(wx, rec)
		return true
	}

	return false
}

func (table Table) valid(tx stm.Txn, rec Record) bool {
	v, err := tx.First(table.records, "id", rec.Peer())
	if v == nil {
		return err == nil
	}

	old := v.(Record)

	// Same instance?  Prefer most recent record.
	if old.Instance() == rec.Instance() {
		return old.Seq() < rec.Seq()
	}

	// Different instance?  Prefer youngest record.  If seq
	// is same, prefer newly-received record.
	return old.Seq() > rec.Seq()
}

func (table Table) upsert(wx stm.Txn, rec Record) {
	if err := wx.Insert(table.records, rec); err != nil {
		panic(err)
	}
}
