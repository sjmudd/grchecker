# grchecker

`grchecker` - a program for monitoring a MySQL Group Replication cluster

If you want to see how your GR cluster is performing this program
may help. It currently behaves a little like `vmstat` in polling a
single server and then showing the state of the GR cluster.

I intend to extend this later to poll all members and then check
for cluster inconsistencies, indicating them appropriately.

## Usage

```
Usage: grchecker <options> [<DSN>] [<interval>]
```

If no parameters are provided then it will try to read from the
`MYSQL_DSN` environment variable to determine the MySQL host to connect
to.  The format to use is the [github.com/go-sql-drivers/mysql](https://github.com/go-sql-driver/mysql)
package's DSN format.

The polling interval is seconds and can be modified.  The default
value is 1 second.

```
Options:
--debug enable debug logging
--interval=<interval> collection interval in seconds. Default 1.
--dsn=<dsn>  golang MySQL DSN to use to connect to a server.
```

## Notes

This is very much work in progress. Current behaviour works on MySQL
8.0 but can collect MySQL 8.2+ GR table information too.

### Contributing

Please file a PR at https://github.com/sjmudd/grchecker

### Licensing

BSD 2-Clause License

### Feedback

Feedback and patches welcome.

Simon J Mudd
<sjmudd@pobox.com>
