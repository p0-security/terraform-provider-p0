package installssh

import installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"

const SshKey = "ssh"

var Components = []string{installresources.IamWrite}
