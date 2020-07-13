package main

import (
	"strings"
	"testing"

	"github.com/kylelemons/godebug/diff"
)

func TestUpgrade(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		expected    string
		expectedErr error
	}{
		{
			name: "no terragrunt attribute",
			input: `
include {
    path = "${find_in_parent_folders()}"
  }
`,
			expected:    "",
			expectedErr: errNotTerragruntConfig,
		},

		{
			name: "simple config",
			input: `
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }

  terraform {
    source = "git::ssh://git@github.com/org/module.git//module?ref=master"
  }
}
`,
			expected: `
include {
  path = find_in_parent_folders()
}

terraform {
  source = "git::ssh://git@github.com/org/module.git//module?ref=master"
}
`,
			expectedErr: nil,
		},

		{
			name: "simple with inputs",
			input: `
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }

  terraform {
    source = "git::ssh://git@github.com/org/module.git//module?ref=master"
  }
}

domain = "app.foo.com"
instance_type = "m5.xlarge"

instance_count = 10
autoscale = true

autoscale_config = {
  min = 5
  max = 15
}

allowed_ports = [80, 443]
`,
			expected: `
include {
  path = find_in_parent_folders()
}

terraform {
  source = "git::ssh://git@github.com/org/module.git//module?ref=master"
}

inputs = {
  domain         = "app.foo.com"
  instance_type  = "m5.xlarge"
  instance_count = 10
  autoscale      = true

  autoscale_config = {
    min = 5
    max = 15
  }

  allowed_ports = [80, 443]
}
`,
			expectedErr: nil,
		},
		{
			name: "complex config",
			input: `/*
* ad-hoc comment
*/

// this will be lost
terragrunt = {
  // this should be preserved

  include {
    path = "${find_in_parent_folders()}"
  }

  # comment
  # with multiple
  # lines
  // and multiple
  // styles...?!
  terraform {
    source = "git::ssh://git@github.com/org/module.git//module?ref=v123" // private repo

    extra_arguments "foo" {
      commands  = ["plan"]
      arguments = ["-var", "foo=bar"]
    }
  }

  // more advanced settings

  dependencies {
    paths = ["./foo"]
  }

  iam_role = "terragrunt-iam-role"
  prevent_destroy = true

  skip = false

  /*
  * remote state settings
  */

  remote_state = {
    backend = "s3"
    config {
      key            = "${path_relative_to_include()}/terraform.tfstate"
      encrypt        = true
      bucket         = "my-tfstate"
      dynamodb_table = "terraform-state-locks"
      region         = "us-east-1"

      s3_bucket_tags {
        name  = "Terraform state storage"
      }

      dynamodb_table_tags {
        name  = "Terraform lock table"
      }
    }
  }
}

# some more comments
# this time it's
# a multi-line comment
domain = "app.foo.com"
instance_type = "m5.xlarge"

instance_count = 10
autoscale = true

// detached between literals

some_other_var = "foo"
another_one = 12

list_var = ["abc", "def", "ghi"]

// here's an ad hoc comment

complex = {
  some_list = ["abc", "def"]
  some_bool = true
  some_int  = 5
  some_str  = "random"

  some_nested_obj = {
    abc = "baz"
  }
}

some_obj_list = [
  {
    foo = "bar"
  },
  {
    baz = "quux"
  },
  {
    quux = <<-EOF
    This is an indented heredoc
    EOF
  },
]

some_heredoc = <<EOF
#!/bin/bash

    echo "here's a shell script"
EOF
`,
			expected: `

/*
* ad-hoc comment
*/

// this should be preserved

include {
  path = find_in_parent_folders()
}

# comment
# with multiple
# lines
// and multiple
// styles...?!
terraform {
  source = "git::ssh://git@github.com/org/module.git//module?ref=v123" // private repo

  extra_arguments "foo" {
    commands  = ["plan"]
    arguments = ["-var", "foo=bar"]
  }
}

// more advanced settings

dependencies {
  paths = ["./foo"]
}

iam_role        = "terragrunt-iam-role"
prevent_destroy = true
skip            = false

/*
  * remote state settings
  */

remote_state {
  backend = "s3"

  config = {
    key            = "${path_relative_to_include()}/terraform.tfstate"
    encrypt        = true
    bucket         = "my-tfstate"
    dynamodb_table = "terraform-state-locks"
    region         = "us-east-1"

    s3_bucket_tags = {
      name = "Terraform state storage"
    }

    dynamodb_table_tags = {
      name = "Terraform lock table"
    }
  }
}

inputs = {
  # some more comments
  # this time it's
  # a multi-line comment
  domain         = "app.foo.com"
  instance_type  = "m5.xlarge"
  instance_count = 10
  autoscale      = true

  // detached between literals

  some_other_var = "foo"
  another_one    = 12
  list_var       = ["abc", "def", "ghi"]

  // here's an ad hoc comment

  complex = {
    some_list = ["abc", "def"]
    some_bool = true
    some_int  = 5
    some_str  = "random"

    some_nested_obj = {
      abc = "baz"
    }
  }

  some_obj_list = [
    {
      foo = "bar"
    },
    {
      baz = "quux"
    },
    {
      quux = <<EOF
This is an indented heredoc
EOF

    },
  ]

  some_heredoc = <<EOF
#!/bin/bash

    echo "here's a shell script"
EOF

}
`,
			expectedErr: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := command{}
			actual, err := cmd.upgrade([]byte(c.input))
			if err != nil && c.expectedErr == nil {
				t.Fatalf("unexpected error: %v", err)
			} else if c.expectedErr != nil && err != c.expectedErr {
				t.Fatalf("incorrect error: got=%v want=%v", err, c.expectedErr)
			}

			// ditch the leading newline - used above to make the formatting a bit nicer
			expected := strings.TrimLeft(c.expected, "\n")
			if string(actual) != expected {
				t.Logf("%q\n\n", actual)
				t.Errorf("incorrect result (-want, +got):\n%s\n", diff.Diff(string(actual), expected))
			}
		})
	}
}
