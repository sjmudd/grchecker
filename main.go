package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

/*
node1 [localhost:21201] {msandbox} (performance_schema) > select * from replication_group_members;
+---------------------------+--------------------------------------+-------------+-------------+--------------+-------------+----------------+----------------------------+
| CHANNEL_NAME              | MEMBER_ID                            | MEMBER_HOST | MEMBER_PORT | MEMBER_STATE | MEMBER_ROLE | MEMBER_VERSION | MEMBER_COMMUNICATION_STACK |
+---------------------------+--------------------------------------+-------------+-------------+--------------+-------------+----------------+----------------------------+
| group_replication_applier | 00021201-1111-1111-1111-111111111111 | 127.0.0.1   |       21201 | OFFLINE      |             |                | MySQL                      |
+---------------------------+--------------------------------------+-------------+-------------+--------------+-------------+----------------+----------------------------+
1 row in set (0.00 sec)
*/
type GroupMember struct {
	channelName              string
	memberID                 string
	memberHost               string
	memberPort               int
	memberState              string
	memberRole               string
	memberVersion            string
	memberCommunicationStack string
}

func (gm GroupMember) String() string {
	return fmt.Sprintf("{channelName: %q, memberID: %q, memberHost: %q:%v, memberState: %q, memberRole: %v, memberVersion: %q, memberCommunicationStack: %q}",
		gm.channelName,
		gm.memberID,
		gm.memberHost,
		gm.memberPort,
		gm.memberState,
		gm.memberRole,
		gm.memberVersion,
		gm.memberCommunicationStack)
}

func getGroupMembers(db *sql.DB) []GroupMember {
	log.Printf("getGroupMembers()\n")
	var gm []GroupMember

	statement := `
SELECT	CHANNEL_NAME, MEMBER_ID, MEMBER_HOST, MEMBER_PORT, MEMBER_STATE, MEMBER_ROLE, MEMBER_VERSION, MEMBER_COMMUNICATION_STACK
FROM	performance_schema.replication_group_members
`
	rows, err := db.Query(statement)
	if err != nil {
		log.Print("query %v failed: %v", statement, err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		member := GroupMember{}

		switch err := rows.Scan(
			&member.channelName,
			&member.memberID,
			&member.memberHost,
			&member.memberPort,
			&member.memberState,
			&member.memberRole,
			&member.memberVersion,
			&member.memberCommunicationStack,
		); err {
		case sql.ErrNoRows:
			log.Printf("error: member: no rows...\n")
		case nil:
		default:
			panic(err)
		}
		gm = append(gm, member)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
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

type GroupMemberActions struct {
	name          string
	event         string
	enabled       int
	actionType    string // can not use type
	priority      int
	errorHandling string
}

func (gcma GroupMemberActions) String() string {
	return fmt.Sprintf("{name: %q, event: %q, enabled: %v, type: %q, priority: %v, errorHandling: %q}",
		gcma.name,
		gcma.event,
		gcma.enabled,
		gcma.actionType,
		gcma.priority,
		gcma.errorHandling)
}

func getGroupMemberActions(db *sql.DB) GroupMemberActions {
	log.Printf("getGroupMemberActions()\n")
	gma := GroupMemberActions{}

	statement := `
SELECT	name, event, enabled, type, priority, error_handling
FROM	performance_schema.replication_group_member_actions
`
	row := db.QueryRow(statement)
	switch err := row.Scan(
		&gma.name,
		&gma.event,
		&gma.enabled,
		&gma.actionType,
		&gma.priority,
		&gma.errorHandling,
	); err {
	case sql.ErrNoRows:
		log.Printf("error: gma: no rows...\n")
	case nil:
	default:
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
	log.Printf("getGroupConfigurationVersion()\n")
	gcv := GroupConfigurationVersion{}

	statement := `
SELECT	name, version
FROM	performance_schema.replication_group_configuration_version
`
	row := db.QueryRow(statement)
	switch err := row.Scan(
		&gcv.name,
		&gcv.version,
	); err {
	case sql.ErrNoRows:
		log.Printf("error: gcv: no rows...\n")
	case nil:
	default:
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

/*
+-------------------+------------------+--------------------------------------+--------------------------------------+---------------------------------------+
| WRITE_CONCURRENCY | PROTOCOL_VERSION | WRITE_CONSENSUS_LEADERS_PREFERRED    | WRITE_CONSENSUS_LEADERS_ACTUAL       | WRITE_CONSENSUS_SINGLE_LEADER_CAPABLE |
+-------------------+------------------+--------------------------------------+--------------------------------------+---------------------------------------+
|                10 | 8.0.27           | a31277bf-6dd8-11ee-8935-fa163ece419e | a31277bf-6dd8-11ee-8935-fa163ece419e |                                     1 |
+-------------------+------------------+--------------------------------------+--------------------------------------+---------------------------------------+
*/

func getGroupCommunicationInformation(db *sql.DB) GroupCommunicationInformation {
	log.Printf("getGroupCommunicationInformation()\n")
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
	switch err := row.Scan(
		&gci.writeConcurrency,
		&gci.protocolVersion,
		&gci.writeConsensusLeadersPreferred,
		&gci.writeConsensusLeadersActual,
		&gci.writeConsensusSingleLeaderCapable,
	); err {
	case sql.ErrNoRows:
		log.Printf("error: gci: no rows...\n")
	case nil:
	default:
		panic(err)
	}
	return gci
}

type CollectionInformation struct {
	collected                  time.Time // when data was collected
	hostname                   string    // expected to be constant but might be behind a lb
	uuid                       string
	mysqlVersion               string
	gtidExecuted               string
	grCommunicationInformation GroupCommunicationInformation
	grConfigurationVersion     GroupConfigurationVersion
	grMemberActions            GroupMemberActions
	grMemberStats              string // fixme!
	grMembers                  []GroupMember
}

func (ci CollectionInformation) String() string {
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

// MemberInformation holds information collected about a member
type MemberInformation struct {
	dsn                   string  // provided host DSN
	db                    *sql.DB // database pool
	firstCollected        time.Time
	lastCollected         time.Time
	updatedCount          int
	collectionInformation *CollectionInformation
}

func (mi *MemberInformation) ConnectIfNotConnected() {
	// connect to database if not already connected
	if mi.db == nil {
		db, err := sql.Open("mysql", mi.dsn)
		if err == nil {
			log.Printf("New connection to database: %q\n", mi.dsn)
			mi.db = db
		} else {
			log.Printf("Failed to connect to database: %q: %v\n", mi.dsn, err)
		}
	}
}

func getBaseInformation(db *sql.DB) (string, string, string, string) {
	log.Printf("getBaseInformation()\n")
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
		log.Printf("error: no rows...\n")
	case nil:
	default:
		panic(err)
	}

	return hostname, uuid, mysqlVersion, gtidExecuted
}

// collect the required details from the database
func (mi *MemberInformation) Collect() *CollectionInformation {
	hostname, uuid, mysqlVersion, gtidExecuted := getBaseInformation(mi.db)

	ci := &CollectionInformation{
		collected:                  time.Now(),
		hostname:                   hostname,
		uuid:                       uuid,
		mysqlVersion:               mysqlVersion,
		gtidExecuted:               gtidExecuted,
		grCommunicationInformation: getGroupCommunicationInformation(mi.db),
		grConfigurationVersion:     getGroupConfigurationVersion(mi.db),
		grMemberActions:            getGroupMemberActions(mi.db),
	}

	return ci
}

func (mi *MemberInformation) Check() {
	log.Printf("mi.Check(%q)\n", mi.dsn)

	mi.ConnectIfNotConnected()

	if mi.db != nil {
		log.Printf("Checking %q...\n", mi.dsn)
		ci := mi.Collect()
		// compare the collected information for changes
		log.Printf("-> %v\n", ci)
		mi.collectionInformation = ci
	} else {
		log.Printf("Skipping checking %q as not connected\n", mi.dsn)
	}
}

// Checker will check the group for changes and inconsistencies
type GroupChecker struct {
	members []*MemberInformation
}

// func (groupChecker *GroupChecker) Check() {
// 	log.Printf("Check(%q)\n", groupChecker)
// }

// NewChecker returns a new Checker
func NewGroupChecker() *GroupChecker {
	log.Printf("NewGroupChecker()\n")
	return &GroupChecker{}
}

// AddMember adds a member to be checked
func (groupChecker *GroupChecker) AddMember(memberDsn string) {
	log.Printf("AddMember(%q)\n", memberDsn)
	groupChecker.members = append(groupChecker.members, &MemberInformation{dsn: memberDsn})
}

// RemoveMember removes a member from the list of members to be checked
func (groupChecker *GroupChecker) RemoveMember(memberDsn string) {
	log.Printf("RemoveMember(%q)\n", memberDsn)
}

// Run starts the checking process
func (groupChecker *GroupChecker) Run() {
	log.Printf("Run()\n")
	for {
		for _, member := range groupChecker.members {
			member.Check()
		}
		time.Sleep(time.Second)
	}
}

func main() {
	var memberDsn string
	if len(os.Args) != 2 {
		log.Fatalln("no dsn provided to connect to")
	}
	memberDsn = os.Args[1]
	if len(memberDsn) == 0 {
		log.Fatalln("empty dsn provided")
	}
	log.Printf("Starting() using dsn: %q\n", memberDsn)

	groupChecker := NewGroupChecker()
	groupChecker.AddMember(memberDsn)
	groupChecker.Run()
	log.Printf("Terminating()\n")
}
