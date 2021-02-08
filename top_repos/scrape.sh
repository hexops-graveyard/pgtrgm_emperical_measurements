set -ex

# 5k repos
githubscrape -min-stars=1000 -language=Go > go.json
githubscrape -min-stars=1000 -language=Java > java.json
githubscrape -min-stars=1000 -language=JavaScript > javascript.json
githubscrape -min-stars=1000 -language=Python > python.json
githubscrape -min-stars=1000 -language=TypeScript > typescript.json

# 5k repos
githubscrape -min-stars=1000 -language=PHP > php.json
githubscrape -min-stars=200 -language=Rust > rust.json # Rust is not as popular as you thought
githubscrape -min-stars=1000 -language=Swift > swift.json
githubscrape -min-stars=1000 -language=C > c.json
githubscrape -min-stars=1000 -language='C++' > cpp.json

# 5k repos
githubscrape -min-stars=700 -language='C#' > csharp.json
githubscrape -min-stars=50 -language=Perl > perl.json # lol perl only has 46 repos with >1,000 stars
githubscrape -min-stars=500 -language=Ruby > ruby.json
githubscrape -min-stars=800 -language=ObjC > objc.json
githubscrape -min-stars=600 -language=sh > sh.json

# 5k repos
githubscrape -min-stars=1000 -language=vb > vb.json
githubscrape -min-stars=20 -language=matlab > matlab.json
githubscrape -min-stars=300 -language=css > css.json
githubscrape -min-stars=500 -language=html > html.json
githubscrape -min-stars=1 -language=solidity > solidity.json

# 578 repos - I <3 Zig
githubscrape -min-stars=0 -language=Zig > zig.json
