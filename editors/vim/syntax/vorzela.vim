" Vorzela Migrate .vm configuration syntax
" Language: vorzela

if exists("b:current_syntax")
  finish
endif

syn case ignore

syn match vorzelaComment /#.*$/ contains=@Spell
syn match vorzelaOperator /=/
syn keyword vorzelaKey DATABASE_URL MIGRATION_PATH SQLC_SUPPORT ENVIRONMENT ENV ENHANCED ONLINE VERIFY_CHECKSUMS DETECT_DRIFT VERBOSE AUTO_RUN_EXTENSIONS AUTO_RUN_FUNCTIONS AUTO_RUN_ENUMS DRIFT_HANDLING
syn match vorzelaBoolean /\v<(true|false|1|0)>/
syn match vorzelaEnvironment /\v<(development|dev|develop|production|prod)>/
syn match vorzelaDrift /\v<(auto|prompt|reject)>/
syn match vorzelaDSN /\v<(postgres(ql)?|mysql):\/\/\S+/
syn match vorzelaTCPDSN /\v\S*@tcp\(\S+/
syn match vorzelaPath /\v\.\/\S+/
syn region vorzelaString start=/"/ skip=/\\./ end=/"/ oneline
syn region vorzelaString start=/'/ skip=/\\./ end=/'/ oneline

hi def link vorzelaComment Comment
hi def link vorzelaKey Keyword
hi def link vorzelaOperator Operator
hi def link vorzelaBoolean Boolean
hi def link vorzelaEnvironment Constant
hi def link vorzelaDrift Constant
hi def link vorzelaDSN String
hi def link vorzelaTCPDSN String
hi def link vorzelaPath String
hi def link vorzelaString String

let b:current_syntax = "vorzela"
