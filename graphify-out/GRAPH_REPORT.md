# Graph Report - .  (2026-06-12)

## Corpus Check
- 132 files · ~136,777 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 2070 nodes · 5303 edges · 93 communities (88 shown, 5 thin omitted)
- Extraction: 76% EXTRACTED · 24% INFERRED · 0% AMBIGUOUS · INFERRED: 1260 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_Branch CLI|Branch CLI]]
- [[_COMMUNITY_Git Errors & Exec|Git Errors & Exec]]
- [[_COMMUNITY_DB & Error Wrapping|DB & Error Wrapping]]
- [[_COMMUNITY_Git Stream Adapter|Git Stream Adapter]]
- [[_COMMUNITY_Branch Health Checks|Branch Health Checks]]
- [[_COMMUNITY_Graph TUI Model|Graph TUI Model]]
- [[_COMMUNITY_Dashboard Adapter Tests|Dashboard Adapter Tests]]
- [[_COMMUNITY_Plugin Manager|Plugin Manager]]
- [[_COMMUNITY_Fake Git Call Records|Fake Git Call Records]]
- [[_COMMUNITY_Fake Git Adapter|Fake Git Adapter]]
- [[_COMMUNITY_Deps Wiring|Deps Wiring]]
- [[_COMMUNITY_Dashboard TUI Model|Dashboard TUI Model]]
- [[_COMMUNITY_Discover Tests|Discover Tests]]
- [[_COMMUNITY_Init CLI|Init CLI]]
- [[_COMMUNITY_Status CLI|Status CLI]]
- [[_COMMUNITY_Log File Rotation|Log File Rotation]]
- [[_COMMUNITY_CLI Chdir Test Harness|CLI Chdir Test Harness]]
- [[_COMMUNITY_Worktree TUI Model|Worktree TUI Model]]
- [[_COMMUNITY_Conventional Commit Parse|Conventional Commit Parse]]
- [[_COMMUNITY_Commit Suggester Wizard|Commit Suggester Wizard]]
- [[_COMMUNITY_Plugin Manifest|Plugin Manifest]]
- [[_COMMUNITY_Branch Wizard|Branch Wizard]]
- [[_COMMUNITY_Log2 Emission Tests|Log2 Emission Tests]]
- [[_COMMUNITY_Plugin CLI Tests|Plugin CLI Tests]]
- [[_COMMUNITY_Init Wizard Tests|Init Wizard Tests]]
- [[_COMMUNITY_Inspect CLI|Inspect CLI]]
- [[_COMMUNITY_Repo Walker|Repo Walker]]
- [[_COMMUNITY_Language Detection|Language Detection]]
- [[_COMMUNITY_Commit Wizard Model|Commit Wizard Model]]
- [[_COMMUNITY_Graph Wizard Tests|Graph Wizard Tests]]
- [[_COMMUNITY_Analyzer Tests|Analyzer Tests]]
- [[_COMMUNITY_Health Scoring|Health Scoring]]
- [[_COMMUNITY_Wizard Key Tests|Wizard Key Tests]]
- [[_COMMUNITY_Graph Render Decorations|Graph Render Decorations]]
- [[_COMMUNITY_DOT Graph Format|DOT Graph Format]]
- [[_COMMUNITY_Wizard TUI Core|Wizard TUI Core]]
- [[_COMMUNITY_Wizard Entry Tests|Wizard Entry Tests]]
- [[_COMMUNITY_Analyzer Core|Analyzer Core]]
- [[_COMMUNITY_Version Parsing|Version Parsing]]
- [[_COMMUNITY_Status Tests|Status Tests]]
- [[_COMMUNITY_Branch Tests|Branch Tests]]
- [[_COMMUNITY_Fake Git Operations|Fake Git Operations]]
- [[_COMMUNITY_Exit Codes|Exit Codes]]
- [[_COMMUNITY_CICD Detection|CICD Detection]]
- [[_COMMUNITY_Init Tests|Init Tests]]
- [[_COMMUNITY_Commit Tests|Commit Tests]]
- [[_COMMUNITY_Git Adapter Types|Git Adapter Types]]
- [[_COMMUNITY_Counting Writer|Counting Writer]]
- [[_COMMUNITY_Counting Writer Tests|Counting Writer Tests]]
- [[_COMMUNITY_Commit Apply|Commit Apply]]
- [[_COMMUNITY_Log Tee Tests|Log Tee Tests]]
- [[_COMMUNITY_Heuristic Suggester|Heuristic Suggester]]
- [[_COMMUNITY_Cmd Log Tests|Cmd Log Tests]]
- [[_COMMUNITY_JSON Helpers & Repo Type|JSON Helpers & Repo Type]]
- [[_COMMUNITY_Graph CLI Tests|Graph CLI Tests]]
- [[_COMMUNITY_Plugin Log Tests|Plugin Log Tests]]
- [[_COMMUNITY_TUI Theme|TUI Theme]]
- [[_COMMUNITY_Remote Log Tests|Remote Log Tests]]
- [[_COMMUNITY_Suggest Tests|Suggest Tests]]
- [[_COMMUNITY_Store SQL Safety|Store SQL Safety]]
- [[_COMMUNITY_Store Tests|Store Tests]]
- [[_COMMUNITY_Session Log|Session Log]]
- [[_COMMUNITY_Detector Tests|Detector Tests]]
- [[_COMMUNITY_Detection Context|Detection Context]]
- [[_COMMUNITY_Failing Fetch Adapter|Failing Fetch Adapter]]
- [[_COMMUNITY_Main Entrypoint|Main Entrypoint]]
- [[_COMMUNITY_Answer Rendering|Answer Rendering]]
- [[_COMMUNITY_Wizard Inputs|Wizard Inputs]]
- [[_COMMUNITY_Fake Adapter Tests|Fake Adapter Tests]]
- [[_COMMUNITY_Wizard Defaults|Wizard Defaults]]
- [[_COMMUNITY_Slog Logger|Slog Logger]]
- [[_COMMUNITY_Wizard Actions|Wizard Actions]]
- [[_COMMUNITY_Plugin Types|Plugin Types]]
- [[_COMMUNITY_Commit Stream Encode|Commit Stream Encode]]
- [[_COMMUNITY_Package Manager Detect|Package Manager Detect]]
- [[_COMMUNITY_Repo Discovery|Repo Discovery]]
- [[_COMMUNITY_Custom Detector|Custom Detector]]
- [[_COMMUNITY_CLI Test Harness|CLI Test Harness]]
- [[_COMMUNITY_Failing Status Adapter|Failing Status Adapter]]
- [[_COMMUNITY_Fake Graph Adapter|Fake Graph Adapter]]
- [[_COMMUNITY_Fake Log Adapter|Fake Log Adapter]]
- [[_COMMUNITY_Wizard Runner|Wizard Runner]]
- [[_COMMUNITY_Detector Core|Detector Core]]
- [[_COMMUNITY_Commit Adapter Types|Commit Adapter Types]]
- [[_COMMUNITY_Merge Adapter Types|Merge Adapter Types]]
- [[_COMMUNITY_Push Adapter Types|Push Adapter Types]]
- [[_COMMUNITY_Worktree Add Adapter Types|Worktree Add Adapter Types]]

## God Nodes (most connected - your core abstractions)
1. `contains()` - 181 edges
2. `NewRootCmd()` - 162 edges
3. `withChdir()` - 146 edges
4. `initGitRepo()` - 121 edges
5. `NoColorTheme()` - 95 edges
6. `FakeAdapter` - 58 edges
7. `String()` - 48 edges
8. `Validation()` - 47 edges
9. `fakeAdapter` - 37 edges
10. `fakeAdapter` - 37 edges

## Surprising Connections (you probably didn't know these)
- `run()` --calls--> `Execute()`  [INFERRED]
  cmd/got/main.go → internal/cli/root.go
- `buildLogger()` --calls--> `NewCountingWriter()`  [INFERRED]
  cmd/got/main.go → internal/log/count.go
- `buildLogger()` --calls--> `Tee()`  [INFERRED]
  cmd/got/main.go → internal/log/log.go
- `buildLogger()` --calls--> `OpenRotatingFile()`  [INFERRED]
  cmd/got/main.go → internal/log/rotate.go
- `buildLogger()` --calls--> `DefaultLevel()`  [INFERRED]
  cmd/got/main.go → internal/log/log.go

## Import Cycles
- None detected.

## Communities (93 total, 5 thin omitted)

### Community 0 - "Branch CLI"
Cohesion: 0.07
Nodes (75): newBranchCheckoutCmd(), newBranchCmd(), newBranchCreateCmd(), newBranchDeleteCmd(), newBranchMoveCmd(), pad(), runBranchCheckout(), runBranchCreate() (+67 more)

### Community 1 - "Git Errors & Exec"
Cohesion: 0.07
Nodes (38): GitError(), Validation(), buildDOTNodeLabel(), encodeCommitsNDJSON(), exitCode(), isStatusChange(), parseBranches(), parseRemotes() (+30 more)

### Community 2 - "DB & Error Wrapping"
Cohesion: 0.06
Nodes (40): DB, Code, Error, New(), NotInitialized(), PermissionDenied(), PluginError(), TestConstructors() (+32 more)

### Community 3 - "Git Stream Adapter"
Cohesion: 0.05
Nodes (35): commitStream, fakeAdapter, fakeEOF, findFinding(), TestCheck_ExcessiveBranches(), TestCheck_LargeBinaries(), TestCheck_LargeBinaries_SkipsNodeModules(), TestCheck_MalformedRemoteURL() (+27 more)

### Community 4 - "Branch Health Checks"
Cohesion: 0.05
Nodes (60): checkExcessiveBranches(), checkMergedBranches(), checkStaleBranches(), defaultBranchFor(), earliestCommitTime(), reachableSHAs(), severityForCount(), Checker (+52 more)

### Community 5 - "Graph TUI Model"
Cohesion: 0.07
Nodes (45): Model, NewModel(), sliceLine(), stringWidth(), state, newTestModel(), TestComposeViewport_HonoursHOffset(), TestNewModel_LinesAreSplit() (+37 more)

### Community 6 - "Dashboard Adapter Tests"
Cohesion: 0.07
Nodes (53): fakeDashboardAdapter, fakeWorktreeAdapter, dashboardDepsFor(), TestTUICmd_GraphPreviewTruncatesTo20(), TestTUICmd_NotInGitRepoFails(), TestTUICmd_NoTUIPrintEmptyStateFriendly(), TestTUICmd_NoTUIPrintsSummary(), TestTUICmd_TTYDrivesWizard() (+45 more)

### Community 7 - "Plugin Manager"
Cohesion: 0.10
Nodes (61): listFilter, filterPlugins(), installFromSource(), loadEnabledSet(), newPluginCmd(), newPluginDisableCmd(), newPluginEnableCmd(), newPluginInfoCmd() (+53 more)

### Community 8 - "Fake Git Call Records"
Cohesion: 0.07
Nodes (24): FakeCheckoutCall, FakeCommitCall, FakeCreateBranchCall, FakeDeleteBranchCall, FakeFetchCall, FakeGraphCall, FakeLogCall, FakeMergeCall (+16 more)

### Community 9 - "Fake Git Adapter"
Cohesion: 0.08
Nodes (14): fakeAdapter, Branch, CheckoutOpts, Context, GraphOpts, LogFormat, MergeOpts, PushOpts (+6 more)

### Community 10 - "Deps Wiring"
Cohesion: 0.10
Nodes (40): Deps, defaultDeps(), defaultDepsWithLogger(), decodeLogLines(), newCapturingLogger(), TestExecAdapter_Logger_LevelFiltering(), TestExecAdapter_Logger_NilNoPanic(), TestExecAdapter_Logger_RecordsFailure() (+32 more)

### Community 11 - "Dashboard TUI Model"
Cohesion: 0.10
Nodes (15): Model, NewModel(), statusItemTitle(), statusListDelegate, statusListItem, tab, tabMeta, Cmd (+7 more)

### Community 12 - "Discover Tests"
Cohesion: 0.14
Nodes (37): T, T, T, itoa(), TestDiscover_DedupesAcrossPathAndRepo(), TestDiscover_PathLookup(), TestDiscover_RejectsBadManifest(), TestDiscover_RejectsExitedNonZero() (+29 more)

### Community 13 - "Init CLI"
Cohesion: 0.09
Nodes (35): AIConfig, applyInit(), newInitCmd(), optsLogFields(), resolveAnswers(), runInit(), initOptions, CommitStyle (+27 more)

### Community 14 - "Status CLI"
Cohesion: 0.11
Nodes (34): loadGOTBlock(), newStatusCmd(), runStatus(), splitStatus(), statusLabel(), writeGitStatusHuman(), writeGOTSectionHuman(), writeJSON() (+26 more)

### Community 15 - "Log File Rotation"
Cohesion: 0.12
Nodes (30): File, FileMode, Mutex, T, OpenRotatingFile(), TestOpenRotatingFile_MasksNonPermissionBits(), TestOpenRotatingFile_RejectsEmptyPath(), TestOpenRotatingFile_RejectsNegativeMaxBytes() (+22 more)

### Community 16 - "CLI Chdir Test Harness"
Cohesion: 0.17
Nodes (36): withChdir(), remoteDepsFor(), TestRemoteCmd_Add(), TestRemoteCmd_AddInvalidName(), TestRemoteCmd_AddInvalidURL(), TestRemoteCmd_Fetch(), TestRemoteCmd_FetchAll(), TestRemoteCmd_FetchAllPrune() (+28 more)

### Community 17 - "Worktree TUI Model"
Cohesion: 0.13
Nodes (14): Model, indexOfString(), NewModel(), nextState(), state, Answers, Cmd, Suggestion (+6 more)

### Community 18 - "Conventional Commit Parse"
Cohesion: 0.15
Nodes (28): isAcronym(), isBreakingFooter(), isConventionalType(), isUpper(), looksLikeFooter(), parseHeader(), splitBodyAndFooters(), splitLines() (+20 more)

### Community 19 - "Commit Suggester Wizard"
Cohesion: 0.20
Nodes (31): NewHeuristicSuggester(), key(), step(), TestAnswers_RenderMinimal(), TestAnswers_RenderWithBreaking(), TestDefaults_DefaultTypeIsFeat(), TestNewModel_HeuristicSuggesterPicksTest(), TestNewModel_HeuristicSuggesterPicksType() (+23 more)

### Community 20 - "Plugin Manifest"
Cohesion: 0.12
Nodes (26): Context, Duration, ManifestCommand, ManifestCommand, T, ManifestArg, DiscoveredPlugin, Discoverer (+18 more)

### Community 21 - "Branch Wizard"
Cohesion: 0.13
Nodes (13): menuChoice, Model, currentBranchName(), describe(), NewModel(), state, Answers, Branch (+5 more)

### Community 22 - "Log2 Emission Tests"
Cohesion: 0.15
Nodes (28): execLogDepsFor(), TestGraphCmd_EmitsStartedFinishedLogs(), TestTUICmd_EmitsStartedAndSummaryLogs(), TestWorktreeCmd_Attach_EmitsStartedLog(), TestWorktreeCmd_List_EmitsStartedFinishedLogs(), TestWorktreeCmd_Prune_EmitsStartedFinishedLogs(), remoteLog2DepsFor(), TestRemoteCmd_Add_EmitsStartedFinishedLogs() (+20 more)

### Community 23 - "Plugin CLI Tests"
Cohesion: 0.20
Nodes (28): pluginDepsFor(), TestPluginCmd_DisableIsIdempotent(), TestPluginCmd_DisableRemovesFromYAML(), TestPluginCmd_EnableAddsToYAML(), TestPluginCmd_EnableRejectsUnknownPlugin(), TestPluginCmd_Info(), TestPluginCmd_InfoUnknownFails(), TestPluginCmd_InstallFromPath() (+20 more)

### Community 24 - "Init Wizard Tests"
Cohesion: 0.25
Nodes (27): key(), sampleBranches(), step(), TestCancelledErrorContract(), TestNewModel_DefaultsToMenu(), TestNewModel_PrePinnedAction(), TestUpdate_CheckoutPickerSetsName(), TestUpdate_ConfirmNoGoesBack() (+19 more)

### Community 25 - "Inspect CLI"
Cohesion: 0.16
Nodes (27): newInspectCmd(), runInspect(), writeCICDSection(), writeContainerizationSection(), writeFrameworksSection(), writeIdentitySection(), writeInspectHeader(), writeInspectHuman() (+19 more)

### Community 26 - "Repo Walker"
Cohesion: 0.09
Nodes (25): New(), NewWithDetectors(), skippedFor(), CICDSystem, LanguageStat, MonorepoInfo, RepositoryModel, RepositoryStats (+17 more)

### Community 27 - "Language Detection"
Cohesion: 0.12
Nodes (23): compoundLanguage(), detectLanguages(), round1(), uniqueStrings(), computeStats(), extOf(), isLikelyEmptyRepoError(), countLines() (+15 more)

### Community 28 - "Commit Wizard Model"
Cohesion: 0.15
Nodes (13): Model, formatCommitStyleLabel(), indexOf(), New(), PrePopulated, state, Answers, Cmd (+5 more)

### Community 29 - "Graph Wizard Tests"
Cohesion: 0.20
Nodes (26): key(), step(), TestDefaults(), TestNew_BranchFallback(), TestNew_DefaultsFromDetected(), TestNew_PrePopulatedAnswers(), TestPrePopulatedCustomGoesToTemplate(), TestPrePopulatedSkipsCommitStyle() (+18 more)

### Community 30 - "Analyzer Tests"
Cohesion: 0.19
Nodes (25): findCICD(), findFramework(), findLanguage(), hasCICD(), hasFramework(), hasLanguage(), hasPackageManager(), TestAnalyze_CICD() (+17 more)

### Community 31 - "Health Scoring"
Cohesion: 0.20
Nodes (24): computeFilteredScore(), filterBySeverity(), filterRecommendationsBySeverity(), newHealthCmd(), parseSeverity(), recountSeverity(), runHealth(), severityRankForFilter() (+16 more)

### Community 32 - "Wizard Key Tests"
Cohesion: 0.26
Nodes (24): sampleInputs(), TestModel_HandleKey_DigitShortcuts(), TestModel_HandleKey_LeftSwitchesBack(), TestModel_HandleKey_LeftWrapsFromStatusToPlugins(), TestModel_HandleKey_QuitOnCtrlC(), TestModel_HandleKey_QuitOnEsc(), TestModel_HandleKey_QuitOnQ(), TestModel_HandleKey_RightSwitchesTab() (+16 more)

### Community 33 - "Graph Render Decorations"
Cohesion: 0.20
Nodes (22): Decoration, DecorationKind, Line, classify(), extractDecorations(), parseLine(), Render(), renderLine() (+14 more)

### Community 34 - "DOT Graph Format"
Cohesion: 0.12
Nodes (19): ParseDotInput(), RenderDOT(), TestDotNode_dotLabel_NoDecorations(), TestDotNode_dotLabel_WithDecorations(), TestParseDotInput_LinearHistory(), TestParseDotInput_MergeCommit(), TestRenderDOT_AdapterErrorPropagates(), TestRenderDOT_EnsuresTrailingNewline() (+11 more)

### Community 35 - "Wizard TUI Core"
Cohesion: 0.16
Nodes (12): Answers, Cmd, Entry, Item, KeyMsg, Msg, PrePopulated, Theme (+4 more)

### Community 36 - "Wizard Entry Tests"
Cohesion: 0.19
Nodes (23): Entry, T, sampleEntries(), TestEntry_DescriptionEmptyWhenNothingSet(), TestEntry_DescriptionHasPathAndLockAndRelative(), TestEntry_FilterValueMatchesTitle(), TestEntry_TitleDetachedFallback(), TestEntry_TitleIncludesLabelAndBranch() (+15 more)

### Community 37 - "Analyzer Core"
Cohesion: 0.13
Nodes (20): applyDetectedItems(), defaultConfidence(), readProjectConfig(), sortLanguages(), upsertCICD(), upsertFramework(), upsertLanguage(), upsertPackageManager() (+12 more)

### Community 38 - "Version Parsing"
Cohesion: 0.29
Nodes (21): cargoVersion(), cleanVersion(), confidenceRank(), detectFrameworks(), detectFromCargoToml(), detectFromComposerJSON(), detectFromConfigFiles(), detectFromDotNet() (+13 more)

### Community 39 - "Status Tests"
Cohesion: 0.20
Nodes (20): fakeAdapter, initDepsForStatus(), keysOf(), TestStatusCmd_CleanRepo(), TestStatusCmd_DirtyRepo(), TestStatusCmd_JSON_NotInitialized(), TestStatusCmd_JSON_WithMeta(), TestStatusCmd_NotInGitRepo() (+12 more)

### Community 40 - "Branch Tests"
Cohesion: 0.28
Nodes (20): branchDepsFor(), TestBranchCmd_Checkout(), TestBranchCmd_Create(), TestBranchCmd_CreateNoFrom(), TestBranchCmd_Delete(), TestBranchCmd_DeleteForce(), TestBranchCmd_DeleteRefusesCurrentBranch(), TestBranchCmd_JSON() (+12 more)

### Community 41 - "Fake Git Operations"
Cohesion: 0.10
Nodes (17): FakeCheckoutCall, FakeCommitCall, FakeCreateBranchCall, FakeDeleteBranchCall, FakeFetchCall, FakeRemoteAddCall, FakeRemoteRemoveCall, FakeRemoteRenameCall (+9 more)

### Community 42 - "Exit Codes"
Cohesion: 0.19
Nodes (18): newCmdWithDiscoverErr(), TestExitCode_NotInGitRepo_AcrossCommands(), TestExitCode_NotInitialized_PluginList(), TestExitCode_PermissionDenied_ReadOnlyDir(), TestExitCode_Plugin_PluginError(), TestExitCode_Usage_UnknownFlag(), TestExitCode_Usage_UnknownSubcommand(), TestExitCode_Validation_AlreadyInitialized() (+10 more)

### Community 43 - "CICD Detection"
Cohesion: 0.18
Nodes (16): detectCICD(), hasGlobMatch(), hasPathPrefix(), hasPrefix(), hasSuffix(), cicdAccum, detectContainerization(), helmChartDirs() (+8 more)

### Community 44 - "Init Tests"
Cohesion: 0.29
Nodes (18): cannedAnswers(), initDepsFor(), readMeta(), TestInitCmd_FlagsOverrideDefaults(), TestInitCmd_ForcePreservesDBContent(), TestInitCmd_FreshRepo_FlagsOverrideWizard(), TestInitCmd_FreshRepo_NonInteractive(), TestInitCmd_FreshRepo_Wizard() (+10 more)

### Community 45 - "Commit Tests"
Cohesion: 0.25
Nodes (17): commitDepsFor(), osWriteFile644(), TestCommitCmd_AllFlag(), TestCommitCmd_AmendFlag(), TestCommitCmd_BreakingViaMessage(), TestCommitCmd_EmptySubjectFails(), TestCommitCmd_MessageFlag(), TestCommitCmd_NotInGitRepoFails() (+9 more)

### Community 46 - "Git Adapter Types"
Cohesion: 0.12
Nodes (18): Adapter, Branch, CheckoutOpts, Commit, CommitOpts, GraphOpts, LogFormat, MergeOpts (+10 more)

### Community 47 - "Counting Writer"
Cohesion: 0.21
Nodes (14): Mutex, Writer, T, NewCountingWriter(), CountingWriter, TestHumanBytes(), TestSessionLog_BytesStatsPlainFile(), TestSessionLog_NilSafe() (+6 more)

### Community 48 - "Counting Writer Tests"
Cohesion: 0.21
Nodes (14): Buffer, Mutex, T, TestCountingWriter_ConcurrentWritesCount(), TestCountingWriter_CountsSuccessfulWrites(), TestCountingWriter_EmptyWriteIsNoCount(), TestCountingWriter_NilReceiverErrors(), TestCountingWriter_NilUnderlyingWriterErrors() (+6 more)

### Community 49 - "Commit Apply"
Cohesion: 0.26
Nodes (16): applyCommit(), bodyAfterFirstLine(), currentStagedPaths(), firstLine(), newCommitCmd(), resolveCommitAnswers(), runCommit(), shortSHA() (+8 more)

### Community 50 - "Log Tee Tests"
Cohesion: 0.25
Nodes (16): T, Tee(), TestNew_EmptyLevelIsError(), TestNew_FormatCaseInsensitive(), TestNew_JSONHandler_RecordsJSON(), TestNew_LevelCaseInsensitive(), TestNew_LevelFiltersRecords(), TestNew_NilWriter() (+8 more)

### Community 51 - "Heuristic Suggester"
Cohesion: 0.22
Nodes (13): HeuristicSuggester, scopeRule, clamp01(), combineReasons(), detectScope(), detectType(), formatN(), itoa() (+5 more)

### Community 52 - "Cmd Log Tests"
Cohesion: 0.30
Nodes (13): cmdLogDepsFor(), TestBranchCmd_Checkout_EmitsStartedFinishedLogs(), TestBranchCmd_Create_EmitsStartedFinishedLogs(), TestBranchCmd_Delete_CurrentBranchEmitsWarnLog(), TestBranchCmd_Delete_EmitsStartedFinishedLogs(), TestBranchCmd_List_EmitsStartedFinishedLogs(), TestLoggerFor_NilDepsLoggerIsNoop(), TestStatusCmd_EmitsStartedFinishedLogs() (+5 more)

### Community 53 - "JSON Helpers & Repo Type"
Cohesion: 0.35
Nodes (11): jsonUnmarshal(), classifyRepository(), hasAnySource(), isConfigOnly(), isDocSite(), isDocsOnly(), isLibrary(), isTool() (+3 more)

### Community 54 - "Graph CLI Tests"
Cohesion: 0.32
Nodes (12): graphDepsFor(), TestGraphCmd_DotOutput(), TestGraphCmd_DotOutputNoTrailingNewlineGetsOne(), TestGraphCmd_FilterFlags(), TestGraphCmd_NotInGitRepoFails(), TestGraphCmd_NoTUIPlainOutput(), TestGraphCmd_WizardNoTUIForcesPlainPath(), TestGraphCmd_WizardPathStubbed() (+4 more)

### Community 55 - "Plugin Log Tests"
Cohesion: 0.28
Nodes (12): pluginLoggerDepsFor(), TestPluginCmd_Disable_EmitsStartedFinishedLogs(), TestPluginCmd_Enable_EmitsStartedFinishedLogs(), TestPluginCmd_Enable_UnknownEmitsWarnLog(), TestPluginCmd_Install_EmitsStartedFinishedLogs(), TestPluginCmd_Install_FailureEmitsWarnLog(), TestPluginLogger_NilDepsLoggerIsNoop(), Adapter (+4 more)

### Community 56 - "TUI Theme"
Cohesion: 0.26
Nodes (9): Theme, Time, Action, Answers, Entry, PrePopulated, formatRelative(), joinComma() (+1 more)

### Community 57 - "Remote Log Tests"
Cohesion: 0.32
Nodes (11): remoteLogDepsFor(), TestRemoteCmd_Fetch_EmitsStartedFinishedLogs(), TestRemoteCmd_Fetch_FailureEmitsWarnLog(), TestRemoteCmd_FetchAll_EmitsStartedFinishedLogs(), TestRemoteCmd_Push_EmitsStartedFinishedLogs(), TestRemoteCmd_Push_NonFastForwardEmitsWarnLog(), TestRemoteLogger_NilDepsLoggerIsNoop(), Adapter (+3 more)

### Community 58 - "Suggest Tests"
Cohesion: 0.30
Nodes (11): TestSuggest_AllBuildFiles_PicksBuildType(), TestSuggest_AllCLIFiles(), TestSuggest_AllDocs_PicksDocsType(), TestSuggest_AllTests_PicksTestType(), TestSuggest_ConfidenceIsClampedTo01(), TestSuggest_DefaultsToFeatForUnknown(), TestSuggest_ManyCodeFiles_PicksRefactor(), TestSuggest_MixedScopes_HasLowerConfidence() (+3 more)

### Community 59 - "Store SQL Safety"
Cohesion: 0.38
Nodes (5): Store, Counts, isAllowedCountTable(), isSafeWhere(), wrapCountErr()

### Community 60 - "Store Tests"
Cohesion: 0.27
Nodes (10): T, fsReadFile(), TestCloseNilSafe(), TestMetaRoundtrip(), TestMigrateBodyContainsCreateTable(), TestMigrateIsIdempotent(), TestMigrationVersionParser(), TestOpenCreatesDBAndRunsMigrations() (+2 more)

### Community 61 - "Session Log"
Cohesion: 0.27
Nodes (6): CountingWriter, File, HumanBytes(), plural(), SessionLog, RotatingFile

### Community 62 - "Detector Tests"
Cohesion: 0.47
Nodes (9): mustWrite(), TestDetectCSharpGlob(), TestDetectDetectsName(), TestDetectEmptyDir(), TestDetectGoProject(), TestDetectMultipleFrameworks(), TestDetectNodeProject(), TestDetectPythonProject() (+1 more)

### Community 63 - "Detection Context"
Cohesion: 0.28
Nodes (4): DetectedItem, DetectionContext, DetectionKind, Detector

### Community 64 - "Failing Fetch Adapter"
Cohesion: 0.31
Nodes (5): failingFetchAdapter, failingPushAdapter, Context, fakeAdapter, PushOpts

### Community 65 - "Main Entrypoint"
Cohesion: 0.28
Nodes (8): Logger, Writer, buildLogger(), main(), run(), DefaultLevel(), TestDefaultLevel(), SessionLog

### Community 66 - "Answer Rendering"
Cohesion: 0.36
Nodes (7): Answers, PrePopulated, buildFooters(), Defaults(), Run(), Suggester, Theme

### Community 67 - "Wizard Inputs"
Cohesion: 0.25
Nodes (8): Inputs, Run(), Branch, Context, DiscoveredPlugin, Remote, Status, Theme

### Community 68 - "Fake Adapter Tests"
Cohesion: 0.50
Nodes (8): NewFake(), TestFakeAdapter_Branches(), TestFakeAdapter_CommitRecords(), TestFakeAdapter_Log(), TestFakeAdapter_Remotes(), TestFakeAdapter_Status(), TestFakeAdapter_StatusErr(), T

### Community 69 - "Wizard Defaults"
Cohesion: 0.33
Nodes (8): Answers, Defaults(), FinalModel(), Run(), Detected, Model, PrePopulated, Theme

### Community 70 - "Slog Logger"
Cohesion: 0.31
Nodes (8): Logger, Writer, Level, Discard(), New(), ParseLevel(), TestDiscard_DropsAllRecords(), TestParseLevel()

### Community 71 - "Wizard Actions"
Cohesion: 0.43
Nodes (7): Action, Answers, PrePopulated, Defaults(), Run(), Branch, Theme

### Community 72 - "Plugin Types"
Cohesion: 0.43
Nodes (6): Context, DiscoveredPlugin, Duration, RawMessage, Manager, Message

### Community 73 - "Commit Stream Encode"
Cohesion: 0.40
Nodes (4): encodeCommit(), commitStream, jsonMarshal(), Commit

### Community 74 - "Package Manager Detect"
Cohesion: 0.60
Nodes (6): detectPackageManagers(), packageManagerFromNpmField(), readRootPackageJSON(), packageManagerRule, packageManager, DetectionContext

### Community 75 - "Repo Discovery"
Cohesion: 0.40
Nodes (4): Repo, Repo, Discover(), Open()

### Community 76 - "Custom Detector"
Cohesion: 0.50
Nodes (3): customDetector, DetectedItem, DetectionContext

### Community 77 - "CLI Test Harness"
Cohesion: 0.40
Nodes (4): setupCLITest(), Adapter, Buffer, Command

### Community 78 - "Failing Status Adapter"
Cohesion: 0.40
Nodes (4): failingStatusAdapter, Context, fakeAdapter, Status

### Community 80 - "Fake Log Adapter"
Cohesion: 0.50
Nodes (3): FakeLogCall, LogFormat, Reader

### Community 81 - "Wizard Runner"
Cohesion: 0.50
Nodes (3): Run(), Context, Theme

### Community 82 - "Detector Core"
Cohesion: 0.83
Nodes (3): Detect(), keysSorted(), Detected

## Knowledge Gaps
- **322 isolated node(s):** `Writer`, `Logger`, `SessionLog`, `Analyzer`, `Context` (+317 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `contains()` connect `Log2 Emission Tests` to `Git Errors & Exec`, `DB & Error Wrapping`, `Git Stream Adapter`, `Branch Health Checks`, `Graph TUI Model`, `Dashboard Adapter Tests`, `Plugin Manager`, `Deps Wiring`, `Discover Tests`, `Log File Rotation`, `CLI Chdir Test Harness`, `Conventional Commit Parse`, `Commit Suggester Wizard`, `Plugin Manifest`, `Plugin CLI Tests`, `Init Wizard Tests`, `Language Detection`, `Graph Wizard Tests`, `Wizard Key Tests`, `Graph Render Decorations`, `DOT Graph Format`, `Wizard Entry Tests`, `Analyzer Core`, `Version Parsing`, `Status Tests`, `Branch Tests`, `Exit Codes`, `CICD Detection`, `Commit Tests`, `Counting Writer`, `Counting Writer Tests`, `Log Tee Tests`, `Heuristic Suggester`, `Cmd Log Tests`, `JSON Helpers & Repo Type`, `Graph CLI Tests`, `Plugin Log Tests`, `Remote Log Tests`, `Suggest Tests`, `Store Tests`, `Detector Tests`, `Fake Adapter Tests`, `Detector Core`?**
  _High betweenness centrality (0.421) - this node is a cross-community bridge._
- **Why does `String()` connect `Git Errors & Exec` to `Branch CLI`, `DB & Error Wrapping`, `Branch Health Checks`, `Dashboard Adapter Tests`, `Dashboard TUI Model`, `Discover Tests`, `Status CLI`, `Log File Rotation`, `Plugin Manifest`, `Plugin CLI Tests`, `Language Detection`, `Analyzer Tests`, `Health Scoring`, `Analyzer Core`, `Version Parsing`, `Status Tests`, `CICD Detection`, `Commit Apply`, `Heuristic Suggester`, `JSON Helpers & Repo Type`, `Store Tests`, `Fake Adapter Tests`?**
  _High betweenness centrality (0.167) - this node is a cross-community bridge._
- **Why does `NewRootCmd()` connect `Plugin CLI Tests` to `Branch CLI`, `Dashboard Adapter Tests`, `Plugin Manager`, `Deps Wiring`, `Init CLI`, `Status CLI`, `CLI Chdir Test Harness`, `Log2 Emission Tests`, `Inspect CLI`, `Health Scoring`, `Status Tests`, `Branch Tests`, `Exit Codes`, `Init Tests`, `Commit Tests`, `Commit Apply`, `Cmd Log Tests`, `Graph CLI Tests`, `Plugin Log Tests`, `Remote Log Tests`, `CLI Test Harness`?**
  _High betweenness centrality (0.104) - this node is a cross-community bridge._
- **Are the 176 inferred relationships involving `contains()` (e.g. with `isK8sManifest()` and `detectFromDotNet()`) actually correct?**
  _`contains()` has 176 INFERRED edges - model-reasoned connections that need verification._
- **Are the 157 inferred relationships involving `NewRootCmd()` (e.g. with `TestBranchCmd_Checkout()` and `TestBranchCmd_Create()`) actually correct?**
  _`NewRootCmd()` has 157 INFERRED edges - model-reasoned connections that need verification._
- **Are the 144 inferred relationships involving `withChdir()` (e.g. with `TestBranchCmd_Checkout()` and `TestBranchCmd_Create()`) actually correct?**
  _`withChdir()` has 144 INFERRED edges - model-reasoned connections that need verification._
- **Are the 117 inferred relationships involving `initGitRepo()` (e.g. with `TestBranchCmd_Checkout()` and `TestBranchCmd_Create()`) actually correct?**
  _`initGitRepo()` has 117 INFERRED edges - model-reasoned connections that need verification._