package main

import "go/types"

// The resolved-graph backward slice: the exact set of nodes whose points-to set
// must be computed to answer pts(receiver). This is what an OPTIMAL demand engine
// materializes — no over-activation — so it is the honest footprint measurement,
// independent of the demand engine's looser writer-finding. It is computed from
// the batch solver's own pts (which resolves the dynamic load/store/dispatch
// edges), so on a converged target it is exact; on a timed-out solve it is a
// lower bound (partial pts).

// buildFeeds returns feeds[v] = the nodes that directly feed v's points-to set in
// the fully-resolved dependency graph.
func (d *demand) buildFeeds(ptsBatch []map[int]bool) [][]int {
	feeds := make([][]int, d.n)
	add := func(dst, src int) {
		if dst >= 0 && src >= 0 {
			feeds[dst] = append(feeds[dst], src)
		}
	}
	for i := range d.b.cs {
		c := d.b.cs[i]
		switch c.f {
		case cCopy, cGep:
			add(c.p, c.q) // p's pts derives from q's
		case cLoad: // p = *q : p is fed by q and by the contents of each o in pts(q)
			add(c.p, c.q)
			for o := range ptsBatch[c.q] {
				add(c.p, o)
			}
		case cStore: // *p = q : each object o in pts(p) is fed by q
			for o := range ptsBatch[c.p] {
				add(o, c.q)
			}
		case cInvoke, cICall:
			for o := range ptsBatch[c.p] {
				ob := d.base[o]
				if ob < 0 {
					continue
				}
				var fn = d.b.objFunc[ob]
				if c.f == cInvoke {
					if t := d.b.objType[ob]; t != nil {
						fn = d.methodOf(t, c.method)
					} else {
						fn = nil
					}
				}
				if fn == nil {
					continue
				}
				ps := d.b.fnParams[fn]
				rr := d.b.fnResults[fn]
				off := 0
				if len(ps) > 0 {
					add(ps[0], o) // receiver object flows to param 0
					off = 1
				}
				for j, a := range c.args {
					if j+off < len(ps) {
						add(ps[j+off], a)
					}
				}
				for j, r := range c.rets {
					if j < len(rr) {
						add(r, rr[j])
					}
				}
			}
		case cTFilter: // dst keeps assignable tagged objects
			for o := range ptsBatch[c.q] {
				if ob := d.base[o]; ob >= 0 {
					if t := d.b.objType[ob]; t != nil && types.AssignableTo(t, c.typ) {
						add(c.p, o)
					}
				}
			}
		case cUntag: // dst gets the payload (contents) of type-exact objects
			for o := range ptsBatch[c.q] {
				if ob := d.base[o]; ob >= 0 {
					if t := d.b.objType[ob]; t != nil && types.Identical(t, c.typ) {
						add(c.p, o)
					}
				}
			}
		}
	}
	return feeds
}

// sliceSize returns the number of nodes in the backward slice of seed over feeds,
// and whether the slice reaches the god object (the flood).
func sliceSize(feeds [][]int, seed, godObj int, mark []int, epoch int) (int, bool) {
	stack := []int{seed}
	mark[seed] = epoch
	size := 0
	reachesGod := false
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		size++
		if v == godObj {
			reachesGod = true
		}
		for _, f := range feeds[v] {
			if mark[f] != epoch {
				mark[f] = epoch
				stack = append(stack, f)
			}
		}
	}
	return size, reachesGod
}
