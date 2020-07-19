# terragrunt-v19-upgrade

This is a tool to upgrade [Terragrunt][1] configurations from version <= 0.18 to version 0.19 according to the [upgrade guide][2]. It also attempts to upgrade the configuration syntax from HCL1 to HCL2. 

## Warnings / Known Issues / Limitations

This tool should be used with some caution. By default, its behavior is destructive: When upgrading a v0.18 configuration (`terraform.tfvars`), the new configuration will be "validated" by parsing it with the [HCL2 parser][3]. If no errors are returned, the new configuration will be [formatted][5] and written to disk in a new file (`terragrunt.hcl`), and the old file will be deleted. This should be used with a VCS (or, take a backup first. But seriously, just use git).

This tool does not try to be as comprehensive as the `terraform 0.12upgrade` tool. This should be ok, since the scope of this is much narrower. We're only concerned with upgrading `tfvars` files, and the syntax of those files is much simpler than normal terraform configuration. However, there are still some limitations:

- [Heredoc][4] variables may not be upgraded correctly. If you have heredoc variables in your configuration, check to make sure they were upgraded correctly.
- Whitespace/formatting will not preserved exactly - the upgraded configuration will be formatted with the [standard formatter][5]
- Multi-line comments may not be properly indented after upgrading (see below)
- A "line" or "lead" comment on the `terragrunt` block will be lost (see below)

#### Upgrading comments

For the most part, comments are preserved - including comments that are not "attached" to a specific node in the configuration. The one exception to this is a comment on the `terragrunt` block:

```hcl
// this comment will be lost
terragrunt = { // this comment will also be lost
  // this will be preserved

  // this "detached" comment will also be preserved

  // this will be preserved
  include { // as will this
    path = "${find_in_parent_folders()}" // and this
  }

  /*
  * This comment may not be formatted correctly, but it will be preserved
  */
  remote_state {
    // ...
  }
}

// comments on variables will be preserved as well
```

## Usage

```sh
$ terragrunt-v19-upgrade
usage: terragrunt-v19-upgrade [flags] [file|dir ...]

terragrunt-v19-upgrade -  A tool for upgrading terragrunt configs from terragrunt <= v0.18 to >= v0.19.

Usage: terragrunt-v19-upgrade <command>

Flags:

  -d, --dry-run    Do not update any files, just print changes to stdout (default: false)
  -k, --keep       Keep old terraform.tfvars files (default: false)
  -m, --git-mv     Update files in place and "git mv terraform.tfvars terragrunt.hcl" (default: false)
  -r, --recursive  Search subdirectores for terraform.tfvars files (default: false)

Commands:

  version  Show the version information.

```

To upgrade a single file, run:

```sh
$ terragrunt-v19-upgrade terraform.tfvars
```

Multiple paths can be specified:

```sh
$ terragrunt-v19-upgrade dir1/terraform.tfvars dir2/terraform.tfvars
```

Or, `terragrunt-v19-upgrade` can search for terragrunt configurations recursively:

```sh
$ terragrunt-v19-upgrade -r dir/
```


[1]: https://github.com/gruntwork-io/terragrunt
[2]: https://github.com/gruntwork-io/terragrunt/blob/master/_docs/migration_guides/upgrading_to_terragrunt_0.19.x.md
[3]: https://pkg.go.dev/github.com/hashicorp/hcl2/hclparse?tab=doc
[4]: https://www.terraform.io/docs/configuration/expressions.html#string-literals
[5]: https://pkg.go.dev/github.com/hashicorp/hcl/v2@v2.6.0/hclwrite?tab=doc#Format
