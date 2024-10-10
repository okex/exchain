package iavl

import (
	"sync"
)

var treeMap *TreeMap
type TreeMap struct {
	mtx sync.RWMutex
	// used for checking whether a tree is saved or not
	mutableTreeSavedMap     map[string]*MutableTree
	totalPpncSize           int64
	evmPpncSize             int64
	accPpncSize             int64
	lastUpdatedVersion      int64
}

func init() {
	treeMap = &TreeMap{
		mutableTreeSavedMap: make(map[string]*MutableTree),
	}
}

func (tm *TreeMap) addNewTree(tree *MutableTree) {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	if _, ok := tm.mutableTreeSavedMap[tree.GetModuleName()]; !ok {
		tm.mutableTreeSavedMap[tree.GetModuleName()] = tree
		tree.ndb.cleanPruningInDB()
		go tree.commitSchedule()
		if EnablePruningHistoryState {
			go tree.pruningSchedule()
		}
	}
}

func (tm *TreeMap) getTree(moduleName string) (tree *MutableTree, ok bool) {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()
	tree, ok = tm.mutableTreeSavedMap[moduleName]
	return
}

// updateMutableTreeMap marks into true when operation of save-version is done
func (tm *TreeMap) updatePpnc(version int64) {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()

	if version == tm.lastUpdatedVersion {
		return
	}
	var size int64 = 0
	for _, tree := range tm.mutableTreeSavedMap {
		ppnc := int64(len(tree.ndb.prePersistNodeCache))
		size += ppnc
		if tree.GetModuleName() == "evm" {
			tm.evmPpncSize = ppnc
		}
		if tree.GetModuleName() == "acc" {
			tm.accPpncSize = ppnc
		}
	}
	tm.totalPpncSize = size
	tm.lastUpdatedVersion = version
}

// resetMap clear the TreeMap, only for test.
func (tm *TreeMap) resetMap() {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	tm.mutableTreeSavedMap = make(map[string]*MutableTree)
}
