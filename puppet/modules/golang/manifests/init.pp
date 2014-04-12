class golang ( $version = "1.2" ) {

  exec { "download-golang":
    command => "/usr/bin/wget -O /usr/local/src/go$version.linux-amd64.tar.gz https://go.googlecode.com/files/go$version.linux-amd64.tar.gz",
    creates => "/usr/local/src/go$version.linux-amd64.tar.gz"
  }

  exec { "remove-previous-version":
    command => "/bin/rm -r /usr/local/go",
    onlyif => "/usr/bin/test -d /usr/local/go",
    before => Exec["unarchive-golang-tools"]
  }

  exec { "unarchive-golang-tools":
    command => "/bin/tar -C /usr/local -xzf /usr/local/src/go$version.linux-amd64.tar.gz",
    require => Exec["download-golang"]
  }

  exec { "setup-path":
    command => "/bin/echo 'export PATH=/vagrant/bin:/usr/local/go/bin:\$PATH' >> /home/vagrant/.profile",
    unless => "/bin/grep -q /usr/local/go /home/vagrant/.profile ; /usr/bin/test $? -eq 0"
  }

  exec { "setup-workspace":
    command => "/bin/echo 'export GOPATH=/vagrant' >> /home/vagrant/.profile",
    unless => "/bin/grep -q GOPATH /home/vagrant/.profile ; /usr/bin/test $? -eq 0"
  }

}
