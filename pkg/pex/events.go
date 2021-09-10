package pex

// EvtViewUpdated is emitted on a the host's event bus when
// a gossip round modifies the current view.
type EvtViewUpdated View

func (ev EvtViewUpdated) Loggable() map[string]interface{} {
	return View(ev).Loggable()
}
