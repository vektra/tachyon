
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
