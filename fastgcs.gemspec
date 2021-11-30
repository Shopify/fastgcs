require_relative 'lib/fastgcs/version'

Gem::Specification.new do |spec|
  spec.name          = "fastgcs"
  spec.version       = FastGCS::VERSION
  spec.authors       = ["Burke Libbey"]
  spec.email         = ["burke.libbey@shopify.com"]

  spec.summary       = %q{Access files in GCS buckets... faster...}
  spec.description   = %q{Access files in GCS buckets... faster than gsutil or the official library}
  spec.homepage      = "https://github.com/Shopify/fastgcs"
  spec.license       = "MIT"
  spec.required_ruby_version = Gem::Requirement.new(">= 2.3.0")

  spec.metadata["homepage_uri"] = spec.homepage

  # Specify which files should be added to the gem when it is released.
  # The `git ls-files -z` loads the files in the RubyGem that have been added into git.
  spec.files         = Dir.chdir(File.expand_path('..', __FILE__)) do
    `git ls-files -z`.split("\x0").reject { |f| f.match(%r{^(test|spec|features)/}) }
  end
  spec.bindir        = "exe"
  spec.executables   = spec.files.grep(%r{^exe/}) { |f| File.basename(f) }
  spec.require_paths = ["lib"]
end
