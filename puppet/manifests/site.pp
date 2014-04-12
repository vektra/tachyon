class { "golang":
  version => "1.2"
}

package { "git":
  ensure => "present",
}

package { "rake":
  ensure => "present",
}
