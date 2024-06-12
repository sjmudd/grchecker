package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	defaultInterval                     = 1 // period of collecting information
	expectedGroupReplicationChannelName = "group_replication_applier"
	invalidID                           = -1
	noneString                          = "<None>" // for no transactions processed yet
	hiddenPasswordString                = "<removed>"
)

// global variables are not ideal but leave this for now
var (
	debug    bool
	dsn      string
	interval int
)

func debugLogging(fmt string, args ...any) {
	if debug {
		log.Printf(fmt, args...)
	}
}

/*
node1 [localhost:21201] {msandbox} (performance_schema) > show create table replication_group_member_stats\G
*************************** 1. row ***************************
       Table: replication_group_member_stats
Create Table: CREATE TABLE `replication_group_member_stats` (
  `CHANNEL_NAME` char(64) NOT NULL,
  `VIEW_ID` char(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `MEMBER_ID` char(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `COUNT_TRANSACTIONS_IN_QUEUE` bigint unsigned NOT NULL,
  `COUNT_TRANSACTIONS_CHECKED` bigint unsigned NOT NULL,
  `COUNT_CONFLICTS_DETECTED` bigint unsigned NOT NULL,
  `COUNT_TRANSACTIONS_ROWS_VALIDATING` bigint unsigned NOT NULL,
  `TRANSACTIONS_COMMITTED_ALL_MEMBERS` longtext NOT NULL,
  `LAST_CONFLICT_FREE_TRANSACTION` text NOT NULL,
  `COUNT_TRANSACTIONS_REMOTE_IN_APPLIER_QUEUE` bigint unsigned NOT NULL,
  `COUNT_TRANSACTIONS_REMOTE_APPLIED` bigint unsigned NOT NULL,
  `COUNT_TRANSACTIONS_LOCAL_PROPOSED` bigint unsigned NOT NULL,
  `COUNT_TRANSACTIONS_LOCAL_ROLLBACK` bigint unsigned NOT NULL
) ENGINE=PERFORMANCE_SCHEMA DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
1 row in set (0.00 sec)

*/

// MemberStats holds the content of replication_group_member_stats
type MemberStats struct {
	channelName                           string
	viewID                                string
	memberID                              string
	countTransactionsInQueue              int64
	countTransactionsChecked              int64
	countConflictsDetected                int64
	countTransactionsRowsValidating       int64
	transactionsCommittedAllMembers       string // large GTID set!
	lastConflictFreeTransaction           string
	countTransactionsRemoteInApplierQueue int64
	countTransactionsRemoteApplied        int64
	countTransactionsLocalProposed        int64
	countTransactionsLocalRollback        int64
}

// filteredChannelName shows the field name and value if it's NOT the expected value
func filteredChannelName(name string) string {
	var finalName string

	if name != expectedGroupReplicationChannelName {
		finalName = fmt.Sprintf("channel: %q, ", name)
	}

	return finalName
}

// String returns a representation of a ReplicationGroupMemberStats row
func (ms MemberStats) String() string {
	// channelName may be hidden!
	return fmt.Sprintf("%vview: %q, member: %q, InQueue: %v, Checked: %v, conflicts: %v, rowsValidating: %v, committedAllMembers: %q, lastTransaction: %q, RemoteInApplierQueue: %v, RemoteApplied: %v, LocalProposed: %v, LocalRollback: %v",
		filteredChannelName(ms.channelName),
		ms.viewID,
		ms.memberID,
		ms.countTransactionsInQueue,
		ms.countTransactionsChecked,
		ms.countConflictsDetected,
		ms.countTransactionsRowsValidating,
		"<skipped>", /* ms.transactionsCommittedAllMembers */
		ms.lastConflictFreeTransaction,
		ms.countTransactionsRemoteInApplierQueue,
		ms.countTransactionsRemoteApplied,
		ms.countTransactionsLocalProposed,
		ms.countTransactionsLocalRollback,
	)
}

// getMemberStats returns a slice of MemberStats rows
func getMemberStats(db *sql.DB) []MemberStats {
	debugLogging("getMemberStats()")
	var ms []MemberStats

	statement := `
SELECT	CHANNEL_NAME,
	VIEW_ID,
	MEMBER_ID,
	COUNT_TRANSACTIONS_IN_QUEUE,
	COUNT_TRANSACTIONS_CHECKED,
	COUNT_CONFLICTS_DETECTED,
	COUNT_TRANSACTIONS_ROWS_VALIDATING,
	TRANSACTIONS_COMMITTED_ALL_MEMBERS,
	LAST_CONFLICT_FREE_TRANSACTION,
	COUNT_TRANSACTIONS_REMOTE_IN_APPLIER_QUEUE,
	COUNT_TRANSACTIONS_REMOTE_APPLIED,
	COUNT_TRANSACTIONS_LOCAL_PROPOSED,
	COUNT_TRANSACTIONS_LOCAL_ROLLBACK
FROM	performance_schema.replication_group_member_stats
`
	rows, err := db.Query(statement)
	if err != nil {
		log.Printf("query %v failed: %v", statement, err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		stats := MemberStats{}

		switch err := rows.Scan(
			&stats.channelName,
			&stats.viewID,
			&stats.memberID,
			&stats.countTransactionsInQueue,
			&stats.countTransactionsChecked,
			&stats.countConflictsDetected,
			&stats.countTransactionsRowsValidating,
			&stats.transactionsCommittedAllMembers,
			&stats.lastConflictFreeTransaction,
			&stats.countTransactionsRemoteInApplierQueue,
			&stats.countTransactionsRemoteApplied,
			&stats.countTransactionsLocalProposed,
			&stats.countTransactionsLocalRollback,
		); err {
		case sql.ErrNoRows:
			log.Printf("error: stats: no rows...")
		case nil:
		default:
			panic(err)
		}
		ms = append(ms, stats)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return ms
}

/*
node1 [localhost:21201] {msandbox} (performance_schema) > select * from replication_group_members;
+---------------------------+--------------------------------------+-------------+-------------+--------------+-------------+----------------+----------------------------+
| CHANNEL_NAME              | MEMBER_ID                            | MEMBER_HOST | MEMBER_PORT | MEMBER_STATE | MEMBER_ROLE | MEMBER_VERSION | MEMBER_COMMUNICATION_STACK |
+---------------------------+--------------------------------------+-------------+-------------+--------------+-------------+----------------+----------------------------+
| group_replication_applier | 00021201-1111-1111-1111-111111111111 | 127.0.0.1   |       21201 | OFFLINE      |             |                | MySQL                      |
+---------------------------+--------------------------------------+-------------+-------------+--------------+-------------+----------------+----------------------------+
1 row in set (0.00 sec)
*/

// ReplicationGroupMember contains the content of a single row of the replication_group_members table
type ReplicationGroupMember struct {
	channelName              string
	memberID                 string
	memberHost               string
	memberPort               uint16
	memberState              string
	memberRole               string
	memberVersion            string
	memberCommunicationStack string
}

// hostPort returns the host port into a host:port format, but if host is empty show empty quotes for clarity
func hostPort(host string, port uint16) string {
	h := ""
	if host == "" {
		h = `""`
	} else {
		h = host
	}
	return fmt.Sprintf("%v:%v", h, port)
}

// String returns the representation of a ReplicationGroupMember
func (gm ReplicationGroupMember) String() string {
	// channelName may be hidden!
	return fmt.Sprintf("%vmember: %v (%v), state: %q, role: %v, version: %q, stack: %q",
		filteredChannelName(gm.channelName),
		hostPort(gm.memberHost, gm.memberPort),
		gm.memberID,
		gm.memberState,
		gm.memberRole,
		gm.memberVersion,
		gm.memberCommunicationStack)
}

// getReplicationGroupMembers returns the content of the replication_group_members table
func getReplicationGroupMembers(db *sql.DB) []ReplicationGroupMember {
	debugLogging("getReplicationGroupMembers()")
	var gm []ReplicationGroupMember

	statement := `
SELECT	CHANNEL_NAME, MEMBER_ID, MEMBER_HOST, MEMBER_PORT, MEMBER_STATE, MEMBER_ROLE, MEMBER_VERSION, MEMBER_COMMUNICATION_STACK
FROM	performance_schema.replication_group_members
`
	rows, err := db.Query(statement)
	if err != nil {
		log.Printf("query %v failed: %v", statement, err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		member := ReplicationGroupMember{}

		var memberPort sql.NullInt32

		switch err := rows.Scan(
			&member.channelName,
			&member.memberID,
			&member.memberHost,
			&memberPort,
			&member.memberState,
			&member.memberRole,
			&member.memberVersion,
			&member.memberCommunicationStack,
		); err {
		case sql.ErrNoRows:
			log.Printf("error: member: no rows...")
		case nil:
		default:
			panic(err)
		}
		if memberPort.Valid {
			member.memberPort = uint16(memberPort.Int32)
		}
		gm = append(gm, member)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return gm
}

/*
root@node1 [performance_schema]> select * from replication_group_member_actions;
+------------------------------------------+------------------------+---------+----------+----------+----------------+
| name                                     | event                  | enabled | type     | priority | error_handling |
+------------------------------------------+------------------------+---------+----------+----------+----------------+
| mysql_disable_super_read_only_if_primary | AFTER_PRIMARY_ELECTION |       1 | INTERNAL |        1 | IGNORE         |
| mysql_start_failover_channels_if_primary | AFTER_PRIMARY_ELECTION |       1 | INTERNAL |       10 | CRITICAL       |
+------------------------------------------+------------------------+---------+----------+----------+----------------+
*/

// ReplicationGroupMemberAction contains a row of the table replication_group_member_actions
type ReplicationGroupMemberAction struct {
	name          string
	event         string
	enabled       int
	actionType    string // can not use type
	priority      int
	errorHandling string
}

func (gcma ReplicationGroupMemberAction) String() string {
	return fmt.Sprintf("{name: %q, event: %q, enabled: %v, type: %q, priority: %v, errorHandling: %q}",
		gcma.name,
		gcma.event,
		gcma.enabled,
		gcma.actionType,
		gcma.priority,
		gcma.errorHandling)
}

// return the replication group member actions (if possible)
func getReplicationGroupMemberActions(db *sql.DB) []ReplicationGroupMemberAction {
	debugLogging("getReplicationGroupMemberActions()")
	var actions []ReplicationGroupMemberAction

	statement := `
SELECT	name, event, enabled, type, priority, error_handling
FROM	performance_schema.replication_group_member_actions
`

	rows, err := db.Query(statement)
	if err != nil {
		log.Printf("getReplicationGroupMemberActions: %v", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		addRow := true // be explicit
		gma := ReplicationGroupMemberAction{}

		err := rows.Scan(
			&gma.name,
			&gma.event,
			&gma.enabled,
			&gma.actionType,
			&gma.priority,
			&gma.errorHandling,
		)
		if err == sql.ErrNoRows {
			addRow = false
		} else if missingTable(err) {
			addRow = false
		} else if err != nil {
			// handle error
			panic(err)
		}
		if addRow {
			actions = append(actions, gma)
		}
	}

	return actions
}

// GroupConfigurationVersion contains the content of the replication_group_configuration_version table
type GroupConfigurationVersion struct {
	name    string
	version int
}

func (gcv GroupConfigurationVersion) String() string {
	return fmt.Sprintf("{name: %q, version: %d}", gcv.name, gcv.version)
}

/*
root@node1 [performance_schema]> select * from replication_group_configuration_version;
+----------------------------------+---------+
| name                             | version |
+----------------------------------+---------+
| replication_group_member_actions |       1 |
+----------------------------------+---------+
*/

func getGroupConfigurationVersion(db *sql.DB) GroupConfigurationVersion {
	debugLogging("getGroupConfigurationVersion()")
	gcv := GroupConfigurationVersion{}

	statement := `
SELECT	name, version
FROM	performance_schema.replication_group_configuration_version
`
	row := db.QueryRow(statement)
	err := row.Scan(
		&gcv.name,
		&gcv.version,
	)
	if err == sql.ErrNoRows {
		log.Printf("error: gcv: no rows...")
	} else if missingTable(err) {
		// do nothing
	} else if err != nil {
		// unexpected error
		panic(err)
	}
	return gcv
}

// GroupCommunicationInformation contains a row of the replication_group_configuration_version table
type GroupCommunicationInformation struct {
	writeConcurrency                  int
	protocolVersion                   string
	writeConsensusLeadersPreferred    string
	writeConsensusLeadersActual       string
	writeConsensusSingleLeaderCapable int
}

func (gci GroupCommunicationInformation) String() string {
	return fmt.Sprintf("{writeConcurrency: %d, protocolVersion: %q, writeConsensusLeadersPreferred: %q, writeConsensusLeadersActual: %q, writeConsensusSingleLeaderCapable: %d}",
		gci.writeConcurrency,
		gci.protocolVersion,
		gci.writeConsensusLeadersPreferred,
		gci.writeConsensusLeadersActual,
		gci.writeConsensusSingleLeaderCapable)
}

// panic: Error 1146 (42S02): Table 'performance_schema.replication_group_communication_information' doesn't exist
func missingTable(err error) bool {
	if mysqlError, ok := err.(*mysql.MySQLError); ok {
		if mysqlError.Number == 1146 /* make constant */ && string(mysqlError.SQLState[:]) == "42S02" /* handle better */ {
			return true
		}
	}

	return false
}

/*

not in 8.0.32

+-------------------+------------------+--------------------------------------+--------------------------------------+---------------------------------------+
| WRITE_CONCURRENCY | PROTOCOL_VERSION | WRITE_CONSENSUS_LEADERS_PREFERRED    | WRITE_CONSENSUS_LEADERS_ACTUAL       | WRITE_CONSENSUS_SINGLE_LEADER_CAPABLE |
+-------------------+------------------+--------------------------------------+--------------------------------------+---------------------------------------+
|                10 | 8.0.27           | a31277bf-6dd8-11ee-8935-fa163ece419e | a31277bf-6dd8-11ee-8935-fa163ece419e |                                     1 |
+-------------------+------------------+--------------------------------------+--------------------------------------+---------------------------------------+
*/

func getGroupCommunicationInformation(db *sql.DB) GroupCommunicationInformation {
	debugLogging("getGroupCommunicationInformation()")
	gci := GroupCommunicationInformation{}

	statement := `
SELECT	WRITE_CONCURRENCY,
		PROTOCOL_VERSION,
		WRITE_CONSENSUS_LEADERS_PREFERRED,
		WRITE_CONSENSUS_LEADERS_ACTUAL,
		WRITE_CONSENSUS_SINGLE_LEADER_CAPABLE
FROM	performance_schema.replication_group_communication_information
`
	row := db.QueryRow(statement)
	err := row.Scan(
		&gci.writeConcurrency,
		&gci.protocolVersion,
		&gci.writeConsensusLeadersPreferred,
		&gci.writeConsensusLeadersActual,
		&gci.writeConsensusSingleLeaderCapable,
	)
	if err == nil {
		// no error - do nothing
	} else if err == sql.ErrNoRows {
		// most likely due to not being an active GR member
	} else if missingTable(err) {
		// return nothing as table does not exist
	} else {
		// unexpected error
		panic(err)
	}

	return gci
}

// CollectedStatistics contains the collected GR statistics for a host
type CollectedStatistics struct {
	collected                  time.Time // when data was collected
	hostname                   string    // expected to be constant but might be behind a lb
	uuid                       string
	mysqlVersion               string
	gtidExecuted               string
	grCommunicationInformation GroupCommunicationInformation
	grConfigurationVersion     GroupConfigurationVersion
	grMemberActions            []ReplicationGroupMemberAction
	grMemberStats              []MemberStats
	grMembers                  []ReplicationGroupMember
}

func (ci CollectedStatistics) String() string {
	return fmt.Sprintf("collected: %v, hostname: %v, uuid: %v, mysqlVersion: %v, gtidExecuted: %v, grCommunicationInformation: %+v, grConfigurationVersion: %v, grMemberActions: %v, grMemberStats: %v, grMembers: %+v",
		ci.collected,
		ci.hostname,
		ci.uuid,
		ci.mysqlVersion,
		ci.gtidExecuted,
		ci.grCommunicationInformation,
		ci.grConfigurationVersion,
		ci.grMemberActions,
		ci.grMemberStats,
		ci.grMembers,
	)
}

// removePassword removes the password from the DSN if found for display purposes
// user:pass@... -> user:<removed>@...
func removePassword(dsn string) string {

	colonIndex := strings.Index(dsn, ":")
	if colonIndex == -1 {
		return dsn
	}
	atIndex := strings.Index(dsn, "@")
	if atIndex == -1 {
		return dsn
	}
	if colonIndex >= atIndex {
		return dsn
	}

	return dsn[:colonIndex+1] + hiddenPasswordString + dsn[atIndex:]
}

// Member holds information collected from a member
type Member struct {
	dsn                 string  // provided host DSN
	db                  *sql.DB // database pool
	firstCollected      time.Time
	lastCollected       time.Time
	updatedCount        int
	collectedStatistics CollectedStatistics
}

// ConnectIfNotConnected connects to the database if it's not already connected
func (member *Member) ConnectIfNotConnected() {
	// connect to database if not already connected
	if member.db == nil {
		db, err := sql.Open("mysql", member.dsn)
		if err != nil {
			log.Fatalf("Failed to connect to database: %q: %v", removePassword(member.dsn), err)
		}

		// add a Ping() to catch errors early and not later.
		if err := db.Ping(); err != nil {
			db.Close()
			log.Fatalf("Failed to connect and Ping() database: %q: %v", removePassword(member.dsn), err)
		}
		log.Printf("New connection: %q", removePassword(member.dsn))
		member.db = db
	}
}

func getBaseInformation(db *sql.DB) (string, string, string, string) {
	debugLogging("getBaseInformation()")
	statement := `SELECT @@hostname, @@server_uuid, @@version, @@gtid_executed`
	var hostname, uuid, mysqlVersion, gtidExecuted string

	row := db.QueryRow(statement)
	switch err := row.Scan(
		&hostname,
		&uuid,
		&mysqlVersion,
		&gtidExecuted,
	); err {
	case sql.ErrNoRows:
		log.Printf("error: no rows...")
	case nil:
	default:
		panic(err)
	}

	return hostname, uuid, mysqlVersion, gtidExecuted
}

// Collect information about the specified member
func (member *Member) Collect() CollectedStatistics {
	now := time.Now() // use one value for consistency
	if member.firstCollected.IsZero() {
		member.firstCollected = now
	}
	hostname, uuid, mysqlVersion, gtidExecuted := getBaseInformation(member.db)
	member.lastCollected = now
	member.updatedCount++

	return CollectedStatistics{
		collected:                  now,
		hostname:                   hostname,
		uuid:                       uuid,
		mysqlVersion:               mysqlVersion,
		gtidExecuted:               gtidExecuted,
		grCommunicationInformation: getGroupCommunicationInformation(member.db),
		grConfigurationVersion:     getGroupConfigurationVersion(member.db),
		grMemberActions:            getReplicationGroupMemberActions(member.db),
		grMemberStats:              getMemberStats(member.db),
		grMembers:                  getReplicationGroupMembers(member.db),
	}
}

// diffString provides a representation of a diff of 2 strings
func diffString(oldString, newString string) string {
	if oldString == newString {
		return newString
	}
	return fmt.Sprintf("-%q/+%q", oldString, newString)
}

// fmtDuration provides a formatted duration (seconds with 3 decimas)
func fmtDuration(d time.Duration) string {
	return fmt.Sprintf("%d.%03ds", int(d.Seconds()), d.Milliseconds()%1000)
}

// getMemberStatsFrom returns the member stats by matching on uuid
func getMemberStatsFrom(uuid string, memberStats []MemberStats) *MemberStats {
	for _, member := range memberStats {
		if uuid != "" && uuid == member.memberID {
			return &member
		}
	}
	return nil
}

// getTransactionID converts a string of fhe form ebfcb50a-4b36-11ee-8601-d06726ac8630:80249444 to its id part
// return -1 if the id is invalid
func getTransactionID(trx string) int {
	idString := strings.Split(trx, ":")
	if len(idString) < 2 {
		return -1
	}
	id, err := strconv.Atoi(idString[1])
	if err != nil {
		return -1
	}
	return id
}

// Wrapped TransactionID will return <None> if no transaction has been provided. (e.g. -1)
func wrappedTransactionID(trxID int) string {
	if trxID == invalidID {
		return noneString
	}
	return fmt.Sprintf("%d", trxID)
}

func getLatestTransactionID(slice []MemberStats) int {
	latestID := invalidID
	for _, s := range slice {
		id := getTransactionID(s.lastConflictFreeTransaction)
		if id > latestID {
			latestID = id
		}
	}
	return latestID
}

// updateIntSlice updates the given slice if the values don't match providing a format and the difference
func updateIntSlice(values *[]string, oldValue int64, newValue int64, format string) {
	if oldValue != newValue {
		*values = append(*values, fmt.Sprintf(format, newValue-oldValue))
	}
}

// showDiff shows the difference between 2 sets of CollectedStatistics
func showDiff(oldData, newData CollectedStatistics) {
	var (
		info         string
		handledUUIDs []string
	)

	diff := strings.Join([]string{
		fmtDuration(newData.collected.Sub(oldData.collected)),
		diffString(oldData.hostname, newData.hostname),
		diffString(oldData.uuid, newData.uuid),
		diffString(oldData.mysqlVersion, newData.mysqlVersion),
	}, " ")
	log.Printf(diff + "")

	// show all member information including relative stats information if available
	log.Print("- members:")
	if len(newData.grMembers) > 0 {
		for _, member := range newData.grMembers {
			statsData := ""
			newMemberStats := getMemberStatsFrom(member.memberID, newData.grMemberStats)
			if newMemberStats != nil {
				handledUUIDs = append(handledUUIDs, member.memberID)
			}
			oldMemberStats := getMemberStatsFrom(member.memberID, oldData.grMemberStats)
			latestTransactionID := getLatestTransactionID(newData.grMemberStats)

			if oldMemberStats != nil && newMemberStats != nil {
				var values []string

				// generate diff statistics
				// - ignore channelName for now but should check it's the same!
				//	viewID
				if oldMemberStats.viewID != newMemberStats.viewID {
					// "16938409756989404:79"
					// crude:
					values = append(values, fmt.Sprintf("view: -%v/+%v", oldMemberStats.viewID, newMemberStats.viewID))
				}
				//	memberID / uuid as must be the same
				//	countTransactionsInQueue
				updateIntSlice(&values, oldMemberStats.countTransactionsInQueue, newMemberStats.countTransactionsInQueue, "InQ: %+d")
				//	countTransactionsChecked
				updateIntSlice(&values, oldMemberStats.countTransactionsChecked, newMemberStats.countTransactionsChecked, "checked: %+d")
				//	countConflictsDetected
				updateIntSlice(&values, oldMemberStats.countConflictsDetected, newMemberStats.countConflictsDetected, "conflicts: %+d")
				//	countTransactionsRowsValidating
				updateIntSlice(&values, oldMemberStats.countTransactionsRowsValidating, newMemberStats.countTransactionsRowsValidating, "validating: %+d")
				//	transactionsCommittedAllMembers       string // large GTID set!
				//	lastConflictFreeTransaction           string
				//  - only show the id part and ignore the uuid: prefix
				transactionID := getTransactionID(newMemberStats.lastConflictFreeTransaction)
				extra := ""
				if transactionID < latestTransactionID {
					extra = fmt.Sprintf(" (behind: %d)", latestTransactionID-transactionID)
				}
				values = append(values, fmt.Sprintf("lastTrx: %v%v", wrappedTransactionID(transactionID), extra))
				//	countTransactionsRemoteInApplierQueue
				updateIntSlice(&values, oldMemberStats.countTransactionsRemoteInApplierQueue, newMemberStats.countTransactionsRemoteInApplierQueue, "remoteInApplierQueue: %+d")
				//	countTransactionsRemoteApplied
				updateIntSlice(&values, oldMemberStats.countTransactionsRemoteApplied, newMemberStats.countTransactionsRemoteApplied, "remoteApplied: %+d")
				//	countTransactionsLocalProposed
				updateIntSlice(&values, oldMemberStats.countTransactionsLocalProposed, newMemberStats.countTransactionsLocalProposed, "localProposed: %+d")
				//	countTransactionsLocalRollback
				updateIntSlice(&values, oldMemberStats.countTransactionsLocalRollback, newMemberStats.countTransactionsLocalRollback, "localRollback: %+d")

				if len(values) > 0 {
					statsData = strings.Join(values, ", ")
				}
			}

			if len(statsData) != 0 {
				statsData = ", " + statsData
			}

			log.Printf("       - %v%v", member.String(), statsData)
		}
	} else {
		info = newData.hostname + " has no GR members"
		log.Printf("       - " + info)
	}

	// show all member stats which haven't been handled
	printed := false
	if len(newData.grMembers) > 0 {
		for _, stats := range newData.grMemberStats {
			// skip the status if handled already (as part of a member)
			if stats.memberID == "" || slices.Contains(handledUUIDs, stats.memberID) {
				continue
			}
			if !printed {
				log.Print("- stats:")
				printed = true
			}
			log.Printf("       - " + stats.String())
		}
	}
}

// MemberCheck checks a member's configuration
func (member *Member) MemberCheck() {
	debugLogging("mi.Check(%q)", removePassword(member.dsn))

	member.ConnectIfNotConnected()

	if member.db != nil {
		debugLogging("Checking %q...", removePassword(member.dsn))
		cs := member.Collect()

		// compare the collected information for changes
		debugLogging("old: %+v", member.collectedStatistics)
		debugLogging("new: %+v", cs)
		showDiff(member.collectedStatistics, cs)
		member.collectedStatistics = cs
	} else {
		log.Printf("Skipping checking %q as not connected", removePassword(member.dsn))
	}
}

// Checker holds a structure of members to check
type Checker struct {
	debug         bool
	sleepInterval time.Duration
	members       []*Member // we update information of the members
}

// NewChecker returns a new Checker
func NewChecker(debug bool, interval int) *Checker {
	log.Printf("NewChecker(%v,%v)", debug, interval)
	debugLogging("NewChecker()")
	return &Checker{
		debug:         debug,
		sleepInterval: time.Second * time.Duration(interval),
	}
}

// AddMember adds a member to be checked
func (checker *Checker) AddMember(memberDsn string) {
	debugLogging("AddMember(%q)", removePassword(memberDsn))
	checker.members = append(checker.members, &Member{dsn: memberDsn})
}

// RemoveMember removes a member from the list of members to be checked
func (checker *Checker) RemoveMember(memberDsn string) {
	debugLogging("RemoveMember(%q)", removePassword(memberDsn))
}

// Run checks all the members
func (checker *Checker) Run() {
	debugLogging("Run()")
	for {
		for _, member := range checker.members {
			member.MemberCheck()
		}
		time.Sleep(checker.sleepInterval)
	}
}

func main() {
	var memberDsn string

	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.IntVar(&interval, "interval", defaultInterval, "interval to collect metrics")
	flag.StringVar(&dsn, "dsn", "" /* no default dsn */, "MySQL DSN to connect to the (first) server (in the cluster)")
	flag.Parse()

	if len(flag.Args()) > 0 {
		log.Printf("Overrriding dsn %q with %v from command line", dsn, flag.Args()[0])
		memberDsn = flag.Args()[0]
	}
	if len(flag.Args()) > 1 {
		log.Printf("Overrriding interval %v with %v from command line", interval, flag.Args()[1])
		memberDsn = os.Args[1]
	}
	if len(memberDsn) == 0 {
		log.Print("dsn not provided on command line. Trying to retrieve from MYSQL_DSN environment variable")
		memberDsn = os.Getenv("MYSQL_DSN")
	}
	if len(memberDsn) == 0 {
		log.Fatal("no dsn provided, MySQL_DSN not set or dsn provided is empty")
	}

	// update interval
	if len(flag.Args()) >= 3 {
		argInterval, err := strconv.Atoi(flag.Args()[2])
		if err != nil {
			log.Fatalln("unable to convert", flag.Args()[2], "to an int:", err)
		}
		interval = argInterval
	}
	log.Printf("Using dsn: %q", removePassword(memberDsn))

	checker := NewChecker(debug, interval)
	checker.AddMember(memberDsn)
	checker.Run()
}
