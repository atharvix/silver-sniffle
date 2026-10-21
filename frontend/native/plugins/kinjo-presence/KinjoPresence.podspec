Pod::Spec.new do |s|
  s.name = 'KinjoPresence'
  s.version = '1.0.0'
  s.summary = 'Keeps a signed-in Kinjo user visible to people nearby from native code'
  s.license = { :type => 'Proprietary' }
  s.homepage = 'https://kinjo.world'
  s.author = 'Kinjo'
  s.source = { :git => 'https://kinjo.world/kinjo-presence.git', :tag => s.version.to_s }
  s.source_files = 'ios/Sources/**/*.swift'
  s.ios.deployment_target = '15.0'
  s.dependency 'Capacitor'
  s.swift_version = '5.9'
end
