
namespace :build do
  task :host do
    sh "go build cmd/tachyon.go"
  end

  task :linux do
    sh "sh -c 'GOOS=linux GOARCH=amd64 go build -o tachyon-linux-amd64 cmd/tachyon.go'"
  end

  task :all => [:host, :linux]
end

namespace :test do
  task :normal do
    sh "go test -v"
  end

  task :package do
    dir = File.expand_path("~/go")
    sh "sudo GOPATH=#{dir} /usr/local/go/bin/go test ./package/apt -v"
  end
end

task :test => ["test:normal", "test:package"]

task :default => :test
