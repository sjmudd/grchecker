package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// global variables are not ideal but leave this for now
var debug bool

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

type MemberStats struct {
	channelName                           string
	viewID                                string
	memberId                              string
	countTransactionsInQueue              int64
	countTransactionsChecked              int64
	countConflictsDetected                int64
	countTransactionsRowsValidating       int64
	transactionsCommittedAllMembers       string
	lastConflictFreeTransaction           string
	countTransactionsRemoteInApplierQueue int64
	countTransactionsRemoteApplied        int64
	countTransactionsLocalProposed        int64
	countTransactionsLocalRollback        int64
}

func (ms MemberStats) String() string {
	return fmt.Sprintf("{ channelName: %q, viewID: %q, memberID: %q, countTransactionsInQueue: %v, countTransactionsChecked: %v, countConflictsDetected: %v, countTransactionsRowsValidating: %v, transactionsCommittedAllMembers: %q, lastConflictFreeTransaction: %q, countTransactionsRemoteInApplierQueue: %v, countTransactionsRemoteApplied: %v, countTransactionsLocalProposed: %v, countTransactionsLocalRollback: %v}",
		ms.channelName,
		ms.viewID,
		ms.memberId,
		ms.countTransactionsInQueue,
		ms.countTransactionsChecked,
		ms.countConflictsDetected,
		ms.countTransactionsRowsValidating,
		ms.transactionsCommittedAllMembers,
		ms.lastConflictFreeTransaction,
		ms.countTransactionsRemoteInApplierQueue,
		ms.countTransactionsRemoteApplied,
		ms.countTransactionsLocalProposed,
		ms.countTransactionsLocalRollback,
	)
}

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
		log.Print("query %v failed: %v", statement, err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		stats := MemberStats{}

		switch err := rows.Scan(
			&stats.channelName,
			&stats.viewID,
			&stats.memberId,
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

// convert a host, port to host:port, but if host is empty show empty quotes for clarity
func hostPort(host string, port uint16) string {
	h := ""
	if host == "" {
		h = `""`
	} else {
		h = host
	}
	return fmt.Sprintf("%v:%v", h, port)
}

func (gm ReplicationGroupMember) String() string {
	return fmt.Sprintf("{channelName: %q, memberID: %q, memberHost: %v, memberState: %q, memberRole: %v, memberVersion: %q, memberCommunicationStack: %q}",
		gm.channelName,
		gm.memberID,
		hostPort(gm.memberHost, gm.memberPort),
		gm.memberState,
		gm.memberRole,
		gm.memberVersion,
		gm.memberCommunicationStack)
}

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
	log.Printf("- found %d group members", len(gm))
	return gm
}

/*
root@grmetadb-1003 [performance_schema]> select * from replication_group_member_actions;
+------------------------------------------+------------------------+---------+----------+----------+----------------+
| name                                     | event                  | enabled | type     | priority | error_handling |
+------------------------------------------+------------------------+---------+----------+----------+----------------+
| mysql_disable_super_read_only_if_primary | AFTER_PRIMARY_ELECTION |       1 | INTERNAL |        1 | IGNORE         |
| mysql_start_failover_channels_if_primary | AFTER_PRIMARY_ELECTION |       1 | INTERNAL |       10 | CRITICAL       |
+------------------------------------------+------------------------+---------+----------+----------+----------------+
*/

type ReplicationGroupMemberActions struct {
	name          string
	event         string
	enabled       int
	actionType    string // can not use type
	priority      int
	errorHandling string
}

func (gcma ReplicationGroupMemberActions) String() string {
	return fmt.Sprintf("{name: %q, event: %q, enabled: %v, type: %q, priority: %v, errorHandling: %q}",
		gcma.name,
		gcma.event,
		gcma.enabled,
		gcma.actionType,
		gcma.priority,
		gcma.errorHandling)
}

func getReplicationGroupMemberActions(db *sql.DB) ReplicationGroupMemberActions {
	debugLogging("getReplicationGroupMemberActions()")
	gma := ReplicationGroupMemberActions{}

	statement := `
SELECT	name, event, enabled, type, priority, error_handling
FROM	performance_schema.replication_group_member_actions
`
	row := db.QueryRow(statement)
	err := row.Scan(
		&gma.name,
		&gma.event,
		&gma.enabled,
		&gma.actionType,
		&gma.priority,
		&gma.errorHandling,
	)
	if err == sql.ErrNoRows {
		log.Printf("error: gma: no rows...")
	} else if missingTable(err) {
		// do nothing
	} else if err != nil {
		// handle error
		panic(err)
	}
	return gma
}

// seems to have only one row atm
type GroupConfigurationVersion struct {
	name    string
	version int
}

func (gcv GroupConfigurationVersion) String() string {
	return fmt.Sprintf("{name: %q, version: %d}", gcv.name, gcv.version)
}

/*
root@grmetadb-1003 [performance_schema]> select * from replication_group_configuration_version;
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

type CollectedStatistics struct {
	collected                  time.Time // when data was collected
	hostname                   string    // expected to be constant but might be behind a lb
	uuid                       string
	mysqlVersion               string
	gtidExecuted               string
	grCommunicationInformation GroupCommunicationInformation
	grConfigurationVersion     GroupConfigurationVersion
	grMemberActions            ReplicationGroupMemberActions
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

// Member holds information collected from a member
type Member struct {
	dsn                 string  // provided host DSN
	db                  *sql.DB // database pool
	firstCollected      time.Time
	lastCollected       time.Time
	updatedCount        int
	collectedStatistics CollectedStatistics
}

func (mi *Member) ConnectIfNotConnected() {
	// connect to database if not already connected
	if mi.db == nil {
		db, err := sql.Open("mysql", mi.dsn)
		if err == nil {
			log.Printf("New connection to database: %q", mi.dsn)
			mi.db = db
		} else {
			log.Printf("Failed to connect to database: %q: %v", mi.dsn, err)
		}
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

// Collect infprmation about the specified member
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

func diffString(oldString, newString string) string {
	if oldString == newString {
		return newString
	}
	return fmt.Sprintf("-%q/+%q", oldString, newString)
}

func fmtDuration(d time.Duration) string {
	return fmt.Sprintf("%d.%03ds", int(d.Seconds()), d.Milliseconds()%1000)
}

func showDiff(oldData, newData CollectedStatistics) {
	diff := strings.Join([]string{
		//		newData.collected.Sub(oldData.collected).String(),
		fmtDuration(newData.collected.Sub(oldData.collected)),
		diffString(oldData.hostname, newData.hostname),
		diffString(oldData.uuid, newData.uuid),
		diffString(oldData.mysqlVersion, newData.mysqlVersion),
	}, " ")
	log.Printf(diff + "")

	// now show all member information
	var info string
	if len(newData.grMembers) > 0 {
		for _, member := range newData.grMembers {
			info = member.String()
			log.Printf("       - " + info)
		}
	} else {
		info = newData.hostname + " has no GR members"
		log.Printf("       - " + info)
	}
}

func (member *Member) MemberCheck() {
	debugLogging("mi.Check(%q)", member.dsn)

	member.ConnectIfNotConnected()

	if member.db != nil {
		debugLogging("Checking %q...", member.dsn)
		cs := member.Collect()

		log.Printf("DEBUG: collected gr member length: %d", len(cs.grMembers))

		// compare the collected information for changes
		debugLogging("old: %+v", member.collectedStatistics)
		debugLogging("new: %+v", cs)
		showDiff(member.collectedStatistics, cs)
		member.collectedStatistics = cs
	} else {
		log.Printf("Skipping checking %q as not connected", member.dsn)
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
	debugLogging("AddMember(%q)", memberDsn)
	checker.members = append(checker.members, &Member{dsn: memberDsn})
}

// RemoveMember removes a member from the list of members to be checked
func (checker *Checker) RemoveMember(memberDsn string) {
	debugLogging("RemoveMember(%q)", memberDsn)
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
	var (
		memberDsn string
		interval  = 1
	)

	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.Parse()

	if len(os.Args) >= 2 {
		memberDsn = os.Args[1]
	}
	if len(memberDsn) == 0 {
		// dsn not provided or empty so try to get from MYSQL_DSN
		memberDsn = os.Getenv("MYSQL_DSN")
	}
	if len(memberDsn) == 0 {
		log.Fatalln("no dsn provided, MySQL_DSN not set or dsn provided is empty")
	}

	// update interval
	if len(os.Args) >= 3 {
		argInterval, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalln("unable to convert", os.Args[2], "to a int:", err)
		}
		interval = argInterval
	}
	log.Printf("Starting() using dsn: %q", memberDsn)

	checker := NewChecker(debug, interval)
	checker.AddMember(memberDsn)
	checker.Run()
	log.Printf("Terminating()")
}
