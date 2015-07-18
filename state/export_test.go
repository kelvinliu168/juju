// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testcharms"
)

const (
	InstanceDataC      = instanceDataC
	MachinesC          = machinesC
	NetworkInterfacesC = networkInterfacesC
	ServicesC          = servicesC
	SettingsC          = settingsC
	UnitsC             = unitsC
	UsersC             = usersC
	BlockDevicesC      = blockDevicesC
	StorageInstancesC  = storageInstancesC
	StatusesHistoryC   = statusesHistoryC
)

var (
	ToolstorageNewStorage  = &toolstorageNewStorage
	ImageStorageNewStorage = &imageStorageNewStorage
	MachineIdLessThan      = machineIdLessThan
	StateServerAvailable   = &stateServerAvailable
	GetOrCreatePorts       = getOrCreatePorts
	GetPorts               = getPorts
	PortsGlobalKey         = portsGlobalKey
	CurrentUpgradeId       = currentUpgradeId
	NowToTheSecond         = nowToTheSecond
	MultiEnvCollections    = multiEnvCollections
	PickAddress            = &pickAddress
	AddVolumeOp            = (*State).addVolumeOp
	CombineMeterStatus     = combineMeterStatus
	NewStatusNotFound      = newStatusNotFound
)

type (
	CharmDoc        charmDoc
	MachineDoc      machineDoc
	RelationDoc     relationDoc
	ServiceDoc      serviceDoc
	UnitDoc         unitDoc
	BlockDevicesDoc blockDevicesDoc
)

func SetTestHooks(c *gc.C, st *State, hooks ...jujutxn.TestHook) txntesting.TransactionChecker {
	return txntesting.SetTestHooks(c, newMultiEnvRunnerForHooks(st), hooks...)
}

func SetBeforeHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	return txntesting.SetBeforeHooks(c, newMultiEnvRunnerForHooks(st), fs...)
}

func SetAfterHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	return txntesting.SetAfterHooks(c, newMultiEnvRunnerForHooks(st), fs...)
}

func SetRetryHooks(c *gc.C, st *State, block, check func()) txntesting.TransactionChecker {
	return txntesting.SetRetryHooks(c, newMultiEnvRunnerForHooks(st), block, check)
}

func newMultiEnvRunnerForHooks(st *State) jujutxn.Runner {
	runner := newMultiEnvRunner(st.EnvironUUID(), st.db)
	st.transactionRunner = runner
	return getRawRunner(runner)
}

// SetPolicy updates the State's policy field to the
// given Policy, and returns the old value.
func SetPolicy(st *State, p Policy) Policy {
	old := st.policy
	st.policy = p
	return old
}

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

func ServiceSettingsRefCount(st *State, serviceName string, curl *charm.URL) (int, error) {
	settingsRefsCollection, closer := st.getCollection(settingsrefsC)
	defer closer()

	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := settingsRefsCollection.FindId(key).One(&doc); err == nil {
		return doc.RefCount, nil
	}
	return 0, mgo.ErrNotFound
}

func AddTestingCharm(c *gc.C, st *State, name string) *Charm {
	return addCharm(c, st, "quantal", testcharms.Repo.CharmDir(name))
}

func AddTestingService(c *gc.C, st *State, name string, ch *Charm, owner names.UserTag) *Service {
	return addTestingService(c, st, name, ch, owner, nil, nil)
}

func AddTestingServiceWithNetworks(c *gc.C, st *State, name string, ch *Charm, owner names.UserTag, networks []string) *Service {
	return addTestingService(c, st, name, ch, owner, networks, nil)
}

func AddTestingServiceWithStorage(c *gc.C, st *State, name string, ch *Charm, owner names.UserTag, storage map[string]StorageConstraints) *Service {
	return addTestingService(c, st, name, ch, owner, nil, storage)
}

func addTestingService(c *gc.C, st *State, name string, ch *Charm, owner names.UserTag, networks []string, storage map[string]StorageConstraints) *Service {
	c.Assert(ch, gc.NotNil)
	service, err := st.AddService(name, owner.String(), ch, networks, storage)
	c.Assert(err, jc.ErrorIsNil)
	return service
}

func AddCustomCharm(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	path := testcharms.Repo.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		config := filepath.Join(path, filename)
		err := ioutil.WriteFile(config, []byte(content), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	ch, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	if revision != -1 {
		ch.SetRevision(revision)
	}
	return addCharm(c, st, series, ch)
}

func addCharm(c *gc.C, st *State, series string, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%s-%d", series, ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:" + series + "/" + ident)
	sch, err := st.AddCharm(ch, curl, "dummy-path", ident+"-sha256")
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

// SetCharmBundleURL sets the deprecated bundleurl field in the
// charm document for the charm with the specified URL.
func SetCharmBundleURL(c *gc.C, st *State, curl *charm.URL, bundleURL string) {
	ops := []txn.Op{{
		C:      charmsC,
		Id:     st.docID(curl.String()),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"bundleurl", bundleURL}}}},
	}}
	err := st.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

// SCHEMACHANGE
// This method is used to reset the ownertag attribute
func SetServiceOwnerTag(s *Service, ownerTag string) {
	s.doc.OwnerTag = ownerTag
}

// SCHEMACHANGE
// Get the owner directly
func GetServiceOwnerTag(s *Service) string {
	return s.doc.OwnerTag
}

func SetPasswordHash(e Authenticator, passwordHash string) error {
	type hasSetPasswordHash interface {
		setPasswordHash(string) error
	}
	return e.(hasSetPasswordHash).setPasswordHash(passwordHash)
}

// Return the underlying PasswordHash stored in the database. Used by the test
// suite to check that the PasswordHash gets properly updated to new values
// when compatibility mode is detected.
func GetPasswordHash(e Authenticator) string {
	type hasGetPasswordHash interface {
		getPasswordHash() string
	}
	return e.(hasGetPasswordHash).getPasswordHash()
}

func init() {
	txnLogSize = txnLogSizeTests
}

// TxnRevno returns the txn-revno field of the document
// associated with the given Id in the given collection.
func TxnRevno(st *State, coll string, id interface{}) (int64, error) {
	var doc struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	err := st.db.C(coll).FindId(id).One(&doc)
	if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

// MinUnitsRevno returns the Revno of the minUnits document
// associated with the given service name.
func MinUnitsRevno(st *State, serviceName string) (int, error) {
	minUnitsCollection, closer := st.getCollection(minUnitsC)
	defer closer()
	var doc minUnitsDoc
	if err := minUnitsCollection.FindId(serviceName).One(&doc); err != nil {
		return 0, err
	}
	return doc.Revno, nil
}

func ConvertTagToCollectionNameAndId(st *State, tag names.Tag) (string, interface{}, error) {
	return st.tagToCollectionAndId(tag)
}

func RunTransaction(st *State, ops []txn.Op) error {
	return st.runTransaction(ops)
}

// Return the PasswordSalt that goes along with the PasswordHash
func GetUserPasswordSaltAndHash(u *User) (string, string) {
	return u.doc.PasswordSalt, u.doc.PasswordHash
}

func CheckUserExists(st *State, name string) (bool, error) {
	return st.checkUserExists(name)
}

func WatcherMergeIds(st *State, changeset *[]string, updates map[interface{}]bool) error {
	return mergeIds(st, changeset, updates)
}

func WatcherEnsureSuffixFn(marker string) func(string) string {
	return ensureSuffixFn(marker)
}

func WatcherMakeIdFilter(st *State, marker string, receivers ...ActionReceiver) func(interface{}) bool {
	return makeIdFilter(st, marker, receivers...)
}

func NewActionStatusWatcher(st *State, receivers []ActionReceiver, statuses ...ActionStatus) StringsWatcher {
	return newActionStatusWatcher(st, receivers, statuses...)
}

func GetAllUpgradeInfos(st *State) ([]*UpgradeInfo, error) {
	upgradeInfos, closer := st.getCollection(upgradeInfoC)
	defer closer()

	var out []*UpgradeInfo
	var doc upgradeInfoDoc
	iter := upgradeInfos.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		out = append(out, &UpgradeInfo{st: st, doc: doc})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func UserEnvNameIndex(username, envName string) string {
	return userEnvNameIndex(username, envName)
}

func DocID(st *State, id string) string {
	return st.docID(id)
}

func LocalID(st *State, id string) string {
	return st.localID(id)
}

func StrictLocalID(st *State, id string) (string, error) {
	return st.strictLocalID(id)
}

func GetUnitEnvUUID(unit *Unit) string {
	return unit.doc.EnvUUID
}

func GetCollection(st *State, name string) (mongo.Collection, func()) {
	return st.getCollection(name)
}

func GetRawCollection(st *State, name string) (*mgo.Collection, func()) {
	return st.getRawCollection(name)
}

func NewMultiEnvRunnerForTesting(envUUID string, baseRunner jujutxn.Runner) jujutxn.Runner {
	return &multiEnvRunner{
		rawRunner: baseRunner,
		envUUID:   envUUID,
	}
}

func Sequence(st *State, name string) (int, error) {
	return st.sequence(name)
}

// This is a naive environment destruction function, used to test environment
// watching after the client calls DestroyEnvironment and the environ doc is removed.
// It is also used to test annotations.
func RemoveEnvironment(st *State, uuid string) error {
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     uuid,
		Assert: txn.DocExists,
		Remove: true,
	}}
	return st.runTransaction(ops)
}

type MockGlobalEntity struct {
}

func (m MockGlobalEntity) globalKey() string {
	return "globalKey"
}
func (m MockGlobalEntity) Tag() names.Tag {
	return names.NewMachineTag("42")
}

var (
	_                    GlobalEntity = (*MockGlobalEntity)(nil)
	TagToCollectionAndId              = (*State).tagToCollectionAndId
)

func AssertAddressConversion(c *gc.C, netAddr network.Address) {
	addr := fromNetworkAddress(netAddr)
	newNetAddr := addr.networkAddress()
	c.Assert(netAddr, gc.DeepEquals, newNetAddr)

	size := 5
	netAddrs := make([]network.Address, size)
	for i := 0; i < size; i++ {
		netAddrs[i] = netAddr
	}
	addrs := fromNetworkAddresses(netAddrs)
	newNetAddrs := networkAddresses(addrs)
	c.Assert(netAddrs, gc.DeepEquals, newNetAddrs)
}

func AssertHostPortConversion(c *gc.C, netHostPort network.HostPort) {
	hostPort := fromNetworkHostPort(netHostPort)
	newNetHostPort := hostPort.networkHostPort()
	c.Assert(netHostPort, gc.DeepEquals, newNetHostPort)

	size := 5
	netHostsPorts := make([][]network.HostPort, size)
	for i := 0; i < size; i++ {
		netHostsPorts[i] = make([]network.HostPort, size)
		for j := 0; j < size; j++ {
			netHostsPorts[i][j] = netHostPort
		}
	}
	hostsPorts := fromNetworkHostsPorts(netHostsPorts)
	newNetHostsPorts := networkHostsPorts(hostsPorts)
	c.Assert(netHostsPorts, gc.DeepEquals, newNetHostsPorts)
}

type StatusDoc statusDoc

func NewStatusDoc(s StatusDoc) statusDoc {
	return statusDoc(s)
}

func NewHistoricalStatusDoc(s StatusDoc, key string) *historicalStatusDoc {
	sdoc := statusDoc(s)
	return newHistoricalStatusDoc(sdoc, key)
}

func ValidateUnitAgentDocDocSet(status Status, info string) error {
	doc, err := newUnitAgentStatusDoc(status, info, nil)
	if err != nil {
		return err
	}
	return doc.validateSet()
}

var StatusHistory = statusHistory
var UpdateStatusHistory = updateStatusHistory

func EraseUnitHistory(u *Unit) error {
	return u.eraseHistory()
}

func UnitGlobalKey(u *Unit) string {
	return u.globalKey()
}

func UnitAgentGlobalKey(u *UnitAgent) string {
	return u.globalKey()
}
