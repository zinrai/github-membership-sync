# github-membership-sync

Makes the members of a GitHub organization, and of the teams named in a YAML
file, match that file.

`diff` prints what `apply` would change, and `apply` changes it. Nothing else
is read or written: teams are neither created nor configured, and repository
access is left alone.

## Usage

```
github-membership-sync diff \
  -org example \
  -config members.yaml \
  -token-file /run/github-token/token
```

prints the plan, one change per line:

```
~ org bob member -> admin
+ org carol member
~ team platform bob member -> maintainer
+ team platform carol member
- team platform dave
- org dave
```

`apply` takes the same flags, prints the same plan and applies it. To start the
file from what the organization has now:

```
github-membership-sync dump -org example -token-file /run/github-token/token > members.yaml
```

`github-membership-sync <command> -h` lists the flags of each command. The
configuration is read from the command line and nowhere else.

On GitHub Enterprise Server, pass the REST API root that server documents.
`-api-base` is used as written and defaults to `https://api.github.com`. The
GraphQL endpoint is derived from it.

```
-api-base https://ghe.example.jp/api/v3
```

## The file

```yaml
members:
  alice: admin
  bob: member
teams:
  platform:
    alice: maintainer
    bob: member
  empty: {}
```

`members` is every member of the organization, as `admin` or `member`. `teams`
is keyed by team slug, each team as `maintainer` or `member`. Only the teams
listed are managed; any other team is not read or changed.

A file is refused, before any request, when it

- has a key other than `members` and `teams`
- has no admin
- lists a team member who is not in `members`
- lists a team with nothing after it; write `{}` to empty a team
- lists one login twice in different case

## Guarantees

`diff` and `apply` write the plan to stdout and nothing else, so an empty
stdout means the organization already matches the file. Logs and errors go to
stderr.

What was read is counted in the log, for comparing with the counts GitHub
shows: members and owners of the organization, and the direct members and
maintainers of each team. `diff` and `apply` log each count as it is and as the
plan leaves it.

Logins are compared without regard to case, as GitHub does.

A team's members are its own: someone in a team only through a child team is
not read as a member of it.

A plan removing more organization members than `-max-removals` (default 3) is
printed and then refused, by `diff` as well as `apply`, with nothing changed.

Changes are applied in the order printed. Members join the organization before
any team, and leave their teams before the organization, so a run that stops
halfway leaves nobody in a team outside the organization. Running it again
picks up where it stopped.

The token file is read before every request, so a file kept current by another
process can be replaced during a run.

## What this does not do

No teams synchronized from an identity provider. GitHub refuses changes to
them, so leave them out of the file.

## Requires

A token for an organization owner, or for a GitHub App installed on the
organization with the Members organization permission. `diff` and `dump` need
it to read only; `apply` needs read and write. An App's token can be kept in a
file by [github-app-tokend](https://github.com/zinrai/github-app-tokend), for
example.

## License

This project is licensed under the [MIT License](LICENSE).
