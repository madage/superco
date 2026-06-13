export const en = {

  // App

  appTitle: 'CoAether',

  appSubtitle: 'AI Agent Distributed Platform',



  // Auth

  login: 'Login',

  register: 'Register',

  username: 'Username',

  email: 'Email',

  password: 'Password',

  logout: 'Logout',

  alreadyHasAccount: 'Already have an account? Login',

  noAccount: "Don't have an account? Register",

  authFailed: 'Authentication failed',



  // Sidebar

  navNodes: 'Nodes',

  navSessions: 'Sessions',

  navTerminal: 'Terminal',



  // Nodes

  agentNodes: 'Agent Nodes',

  loadingNodes: 'Loading nodes...',

  noNodes: 'No nodes registered. Start an Agent Node to begin.',

  refresh: 'Refresh',

  showOffline: 'Show offline',
  lastSeen: 'Last seen',

  nodeOnline: 'online',

  nodeOffline: 'offline',

  nodeBusy: 'busy',



  // Sessions

  sessions: 'Sessions',

  loadingSessions: 'Loading sessions...',

  noSessions: 'No sessions yet. Create one to get started.',

  workspace: 'Workspace',

  created: 'Created',

  sessionPending: 'pending',

  sessionRunning: 'running',

  sessionPaused: 'paused',

  sessionStatusCompleted: 'completed',

  sessionStatusFailed: 'failed',



  // Create Session

  newSession: 'New Session',

  targetNode: 'Target Node',

  selectNode: 'Select a node...',

  noOnlineNodes: 'No online nodes available',

  workspacePath: 'Workspace Path',

  workspacePlaceholder: '/home/user/project or C:\\Users\\me\\project',

  prompt: 'Prompt',

  promptPlaceholder: 'Describe the task for Claude Code...',

  allFieldsRequired: 'All fields are required',

  creating: 'Creating...',

  startSession: 'Start Session',

  failedToCreate: 'Failed to create session',



  // Terminal

  terminal: 'Terminal',

  session: 'Session',

  none: 'None',

  disconnect: 'Disconnect',

  noActiveSession: 'No active session. Create a session from the Sessions tab first.',

  waitingForSession: 'Waiting for session...',

  sessionCompleted: '[Session completed successfully]',

  sessionFailed: '[Session failed: ',

  unknownError: 'unknown error',



  // Projects

  navProjects: 'Projects',

  projectCreate: 'Create Project',

  projectName: 'Name',

  projectDescription: 'Description',

  projectColor: 'Color',

  projectEmpty: 'No projects yet',

  noProject: 'No Project',



  // Tasks

  navTasks: 'Tasks',

  navTrash: 'Trash',

  taskCreate: 'Create Task',

  taskEdit: 'Edit Task',

  taskDelete: 'Delete',

  taskConfirmDelete: 'Are you sure you want to delete this task?',

  taskStatusTodo: 'Todo',

  taskStatusInProgress: 'In Progress',

  taskStatusBlocked: 'Blocked',

  taskStatusDone: 'Done',

  taskStatusCompleted: 'Completed',
  taskStatusStuck: 'Stuck',
  taskStatusReview: 'Review',

  taskTitle: 'Title',

  taskDescription: 'Description',

  taskStatus: 'Status',

  taskEmpty: 'No tasks yet',

  taskViewKanban: 'Kanban',

  taskViewList: 'List',

  taskProgress: 'Progress',

  taskActions: 'Actions',

  taskRestore: 'Restore',

  taskPermanentDelete: 'Delete Forever',

  taskTrashEmpty: 'Trash is empty',
  defaultProject: 'Default project',
  taskFilterPriority: 'Priority',
  creator: 'Creator',
  taskDelegated: 'Delegated',
  deleteVerifyPrompt: 'Answer the following to confirm:',
  deleteVerifyWrong: 'Wrong answer, try again',

  // Task Detail
  taskDetailSaving: 'Saving...',
  taskDetailUnknown: 'Unknown',
  taskDetailUpdated: 'updated',
  taskDetailNoDescription: 'No description',
  taskDetailSubtasks: 'Subtasks',
  taskDetailComments: 'Comments',
  taskDetailAgentBadge: 'Agent',
  taskDetailAgentReport: 'Agent Report',
  taskDetailOverdue: 'This task is past due',
  taskDetailPriority: 'Priority',
  priorityUrgent: 'Urgent',
  priorityHigh: 'High',
  priorityMedium: 'Medium',
  priorityLow: 'Low',
  taskDetailAssignee: 'Assignee',
  taskDetailUnassigned: 'Unassigned',
  taskDetailDelegatedAssignees: 'Delegated Assignees',
  taskDetailRemoveAssignee: 'Remove assignee',
  taskDetailUser: 'User',
  taskDetailSelect: 'Select...',
  taskDetailAdd: 'Add',
  taskDetailAddAssignee: '+ Add assignee',
  taskDetailAddTag: 'Add tag...',
  taskDetailTags: 'Tags',
  taskDetailDueDate: 'Due Date',
  taskDetailProject: 'Project',
  taskDetailNoProject: 'No project',
  taskDetailParentTask: 'Parent Task',
  taskDetailNoneTopLevel: 'None (top-level)',
  taskDetailCreated: 'Created:',
  taskDetailUpdatedTime: 'Updated:',
  taskDetailClose: 'Close',
  taskDetailDeleteTask: 'Delete task',
  taskDetailConfirmDeleteMsg: 'Are you sure you want to delete this task? This action cannot be undone.',
  taskDetailDeleteCommentHint: 'Click again to confirm',
  taskDetailDeleteCommentTitle: 'Delete comment',

  // Review
  reviewApprove: 'Approve',
  reviewReject: 'Reject',
  reviewComment: 'Review Comment',
  reviewCommentPlaceholder: 'Enter review feedback...',
  reviewApproved: 'Approved',
  reviewRejected: 'Rejected',
  reviewApproveSuccess: 'Task approved',
  reviewRejectSuccess: 'Task rejected',
  reviewAgentLoopCount: 'Agent loops',
  reviewMaxLoops: 'Max loops',
  reviewMeltdown: 'Meltdown',

  // Workflow
  taskWorkflow: 'Workflow',
  taskAgentLoops: 'Agent Loops',
  taskAgentLoopCount: 'Current loop',
  taskMaxAgentLoops: 'Max loops',
  taskCompletionBehavior: 'Completion Behavior',
  completionBehaviorAutoDone: 'Auto Done',
  completionBehaviorAutoReview: 'Auto Review',
  completionBehaviorSampleReview: 'Sample Review',
  completionBehaviorNeedsReview: 'Needs Review',

  // Agents

  agents: 'Agents',

  noAgents: 'No agents found on this node',

  scanAgents: 'Scan',

  scanning: 'Scanning...',

  maxSessions: 'Max sessions',

  agent: 'Agent',

  selectAgent: 'Select an agent...',

  noAgentsOnNode: 'This node has no available agents. Click "Scan" to detect installed agents.',
  assignedAgents: 'Assigned Agents',
  noAssignedAgents: 'No assigned agents',

  enabled: 'Enabled',

  disabled: 'Disabled',

  agentHint: 'Agents are auto-detected from PATH. Toggle to enable/disable.',

  loading: 'Loading',

  optional: 'optional',



  // Agent Profiles

  agentProfiles: 'Agent Profiles',

  createAgent: 'Create Agent',

  editAgent: 'Edit Agent',

  deleteAgent: 'Delete Agent',

  agentName: 'Name',

  agentNamePlaceholder: 'Give your agent a name...',

  agentDescription: 'Description',

  agentDescriptionPlaceholder: 'Describe what this agent does...',

  agentRuntime: 'Runtime',

  selectRuntime: 'Select a runtime...',
  agentNode: 'Node',

  saveAgent: 'Save',

  cancel: 'Cancel',

  confirmDelete: 'Are you sure you want to delete this agent?',

  createSuccess: 'Created',

  updateSuccess: 'Updated',

  deleteSuccess: 'Deleted',

  noProfiles: 'No agents yet. Click "+" to add one.',

  profileDetail: 'Detail',

  profileEdit: 'Edit',



  // Permission Mode

  permissionMode: 'Permission Mode',

  autoMode: 'Auto',

  restrictedMode: 'Restricted',



  // Chat

  chat: 'Chat',

  noMessages: 'No messages yet. Send a message to start the conversation.',

  send: 'Send',

  inputPlaceholder: 'Type a message and press Enter...',

  connecting: 'Connecting...',

  sessionActive: 'Active',

  clear: 'Clear',

  sessionNoActive: 'No active session',

  connected: 'connected',

  offline: 'offline',

  you: 'You',

  system: 'System',



  // Workspaces

  workspaceLabel: 'Workspace',

  manageWorkspaces: 'Manage Workspaces',

  addWorkspace: 'Add Workspace',

  workspaceName: 'Workspace Name',

  workspaceDescription: 'Workspace Description',

  workspaceCreate: 'Create Workspace',

  workspaceDelete: 'Delete Workspace',

  workspaceDeleteConfirm: 'Delete this workspace? Tasks and projects will become unassigned.',

  workspaceDefaultName: 'Default',



  // Language

  switchLang: '中文',



  // Plugins

  navPlugins: 'Plugins',

  plugins: 'Plugins',

  pluginLoading: 'Loading plugins...',

  noPlugins: 'No plugins installed.',

  pluginRunning: 'Running',

  pluginStopped: 'Stopped',

  pluginError: 'Error',

  pluginStarting: 'Starting',

  pluginStopping: 'Stopping',

  pluginStart: 'Start',

  pluginStop: 'Stop',

  pluginReload: 'Reload',

  pluginVersion: 'Version',

  pluginAuthor: 'Author',

  pluginPermissions: 'Permissions',

  pluginHooks: 'Hooks',

  pluginRoutes: 'API Routes',

  pluginUptime: 'Uptime',

  pluginPort: 'Port',

  pluginPid: 'PID',

  pluginNoPerms: 'none declared',

  pluginNoHooks: 'none',

  pluginNoRoutes: 'none',

  pluginNoError: 'No error',

  pluginConfirmStop: 'Are you sure you want to stop {name}?',

  pluginConfirmReload: 'Are you sure you want to reload {name}?',

  pluginInstall: 'Install Plugin',
  pluginInstallUpload: 'Upload ZIP',
  pluginInstallGit: 'Git Repository',
  pluginInstallUploadHint: 'Select a plugin ZIP file to upload and install.',
  pluginInstallGitHint: 'Clone a plugin from a remote Git repository.',
  pluginInstallUrl: 'Git URL',
  pluginInstallUrlPlaceholder: 'https://github.com/user/plugin.git',
  pluginInstallBranch: 'Branch (optional)',
  pluginInstallBranchPlaceholder: 'main',
  pluginInstallButton: 'Install',
  pluginInstalling: 'Installing...',
  pluginInstallSuccess: 'Plugin {name} v{version} installed',
  pluginInstallError: 'Installation failed',
  pluginRemove: 'Remove',
  pluginRemoveConfirm: 'Are you sure you want to remove plugin {name}?',



  // Remote Nodes

  addNode: 'Add Node',

  addNodeTitle: 'Add Remote Node',

  nodeName: 'Node Name',

  nodeNamePlaceholder: 'e.g. my-mac-claude',

  step1: 'Run the command on the target machine (Mac or Windows). The agent-runtime will auto-start on login.',

  generateCommand: 'Generate & Show Command',

  runOnMac: 'macOS (bash)',

  runOnWindows: 'Windows (PowerShell)',

  copyCommand: 'Copy',

  copied: 'Copied!',

  waitingNode: 'Waiting for node to connect...',

  nodeAdded: 'Node connected successfully!',

  nodeRemove: 'Remove',

  nodeRemoveConfirm: 'Are you sure you want to remove this node?',

  nodeStopConfirm: 'Complete the verification to stop this node:',
  nodeStopConfirmWrong: 'Incorrect answer, please try again',
  nodeNoPermission: 'You do not have permission to control this node',
  nodeStart: 'Start',
  nodeStop: 'Stop',
  nodeStarting: 'Starting...',
  nodeStopping: 'Stopping...',
  startCommandTitle: 'Start Node',
  startCommandMac: 'macOS / Linux',
  startCommandWindows: 'Windows (PowerShell)',
  startCommandHint: 'Run this command on the target machine to start the agent:',

  alreadyHasNode: 'You already have an active node. Remove it first.',



  // Comments
  commentPlaceholder: 'Write a comment...',
  commentPost: 'Comment',
  commentEmpty: 'No comments yet',
  commentReply: 'Reply',
  commentDelete: 'Delete',
  commentDeleteConfirm: 'Delete this comment?',
  commentBy: 'commented',

  // Notifications

  notifInbox: 'Notifications',
  notifInvitations: 'Invitations',
  notifEmpty: 'No notifications yet',
  notifMarkAllRead: 'Mark all as read',
  notifTaskAssigned: 'Task assigned',
  notifTaskStatusChanged: 'Status changed',
  notifTaskComment: 'New comment',
  notifTimeAgo: '{time} ago',

  pendingInvitations: 'Pending Invitations',

  invitationFrom: '{name} invited you to {workspace}',

  accept: 'Accept',

  decline: 'Decline',

  noPendingInvitations: 'No pending invitations',

  invitationAccepted: 'Invitation accepted',

  invitationDeclined: 'Invitation declined',



  // Automation Rules

  ruleName: 'Rule Name',

  ruleNamePlaceholder: 'My automation rule',

  ruleDescription: 'Description',

  ruleDescriptionPlaceholder: 'Optional description',

  ruleTrigger: 'Trigger',

  ruleConditions: 'Conditions (JSON)',

  ruleConditionsHint: 'Format: field, op (matches/equals/contains), value',

  ruleActions: 'Actions (JSON)',

  ruleActionsHint: 'Array of {type, value} objects',

  ruleEnabled: 'Enabled',

  ruleCreate: 'Create Rule',

  ruleEdit: 'Edit Rule',

  ruleEmpty: 'No automation rules yet.',

  ruleConfirmDelete: 'Delete this rule?',

  ruleTriggerComment: 'On Comment',

  ruleTriggerStatus: 'On Status Change',

  ruleTriggerAssignee: 'On Assignee Change',

  ruleTriggerCreate: 'On Task Create',

  ruleDisable: 'Disable',

  ruleEnable: 'Enable',

  ruleViewLogs: 'View Logs',

  ruleErrorNameRequired: 'Rule name is required',

  ruleErrorJsonInvalid: 'Invalid JSON in conditions or actions',

  ruleLogTitle: 'Execution Logs',

  ruleLogEmpty: 'No execution logs yet.',

  ruleLogTime: 'Time',

  ruleLogTask: 'Task',

  ruleLogEvent: 'Event',

  ruleLogMatched: 'Matched',

  ruleLogResult: 'Result',

  navAutomation: 'Rules',

  edit: 'Edit',

  delete: 'Delete',

  save: 'Save',

  saving: 'Saving...',

  // Skills
  skillLibrary: 'Skill Library',
  skillCreate: 'Create',
  skillEdit: 'Edit Skill',
  skillExtract: 'Extract',
  skillExtractFromTask: 'Extract Skill from Task',
  skillEmpty: 'No skills yet. Create one or extract from a completed task.',
  skillNameLabel: 'Name',
  skillNamePlaceholder: 'Skill name',
  skillDescriptionLabel: 'Description',
  skillDescriptionPlaceholder: 'Optional description',
  skillContent: 'Content',
  skillContentPlaceholder: 'Skill content / prompt template...',
  skillUsage: 'Used',
  skillFromTask: 'From task',
  skillTaskId: 'Task ID',
  skillTaskIdPlaceholder: 'Enter task ID...',
  navTools: 'Tool Set',
  toolSetTitle: 'Tool Set',
  toolEnabled: 'Enabled',
  toolDisabled: 'Disabled',
  toolLinkedAgents: 'Mounted Agents',
  toolNoAgents: 'No agents mounted',
  toolName_create_sub_task: 'Create Sub-task',
  toolName_propose_decomposition_plan: 'Propose Decomposition Plan',
  toolName_assign_task: 'Assign Task',
  toolName_review_task: 'Review Task',
  toolName_add_comment: 'Add Comment',
  toolName_get_task_detail: 'View Detail',
  toolName_list_sub_tasks: 'List Sub-tasks',
  toolName_update_task_status: 'Update Status',
  toolName_search_agent_profiles: 'Search Agents',
  navSkills: 'Skills',
  navAgentQueue: 'Queue',

  // Agent Profile Enhancement

  systemPrompt: 'System Prompt',

  systemPromptPlaceholder: 'Define the agent\'s role, personality, and expertise...',

  maxConcurrency: 'Max Parallel Tasks',

  abilityTags: 'Ability Tags',

  abilityTagsPlaceholder: 'e.g. frontend, backend, database',

  instructions: 'Behavior Instructions',

  instructionsPlaceholder: 'Define communication style, tone, and behavior guidelines to make the agent more personable...',

  agentLoad: 'Load',

  agentLoadInfo: '{current}/{max} tasks running',

  // Agent Harness fields
  reviewSampleRate: 'Review Sample Rate',
  // Agent Folders
  agentFolders: 'Agent Folders',
  folderCreate: 'New Folder',
  folderName: 'Folder Name',
  folderNamePlaceholder: 'Enter folder name...',
  folderColor: 'Color',
  folderRename: 'Rename',
  folderDelete: 'Delete Folder',
  folderDeleteConfirm: 'Delete this folder? Agents inside will not be deleted.',
  folderUncategorized: 'Uncategorized',
  folderAll: 'All Agents',
  folderAddAgent: 'Add to Folder',
  folderRemoveAgent: 'Remove from Folder',
  folderEmpty: 'No folders yet. Click + to create one.',
  folderAgentCount: '{count} agents',

  reviewSampleRateHint: '0.0 = never review, 1.0 = always review',
  reviewTimeout: 'Review Timeout (min)',
  maxReviewLoops: 'Max Review Loops',
  agentMaxDepth: 'Max Depth',
  agentSkills: 'Skills',
  agentProtocolVersion: 'Protocol',
  agentCapabilities: 'Tool Set',
  agentPermissions: 'Permissions',

  // Agent Queue
  agentQueue: 'Agent Queue',
  agentQueueEmpty: 'No queued tasks.',
  agentQueueQueued: 'Queued',
  agentQueueClaimed: 'Claimed',
  agentQueueProcessing: 'Processing',
  agentQueueCompleted: 'Completed',
  agentQueueFailed: 'Failed',

  // Decomposition Plan
  decompPlanTitle: 'Decomposition Plan',
  decompPlanPending: 'Pending',
  decompPlanApproved: 'Approved',
  decompPlanRejected: 'Rejected',
  decompPlanBy: 'by',
  decompPlanSelectAll: 'Select All',
  decompPlanDeselectAll: 'Deselect All',
  decompPlanApproveSelected: 'Approve Selected',
  decompPlanApproveAll: 'Approve All',
  decompPlanReject: 'Reject',
  decompPlanRejectPlaceholder: 'Enter reason for rejection...',
  decompPlanConfirmReject: 'Confirm Reject',
  decompPlanNoItems: 'No items in plan.',
  decompPlanSummary: 'Summary',

  // Token Management
  tokenManagement: 'Token Management',
  tokenList: 'Tokens',
  tokenCreate: 'Generate Token',
  tokenName: 'Name',
  tokenNamePlaceholder: 'Enter token name...',
  tokenExpiry: 'Expiry',
  tokenExpiry7d: '7 Days',
  tokenExpiry30d: '30 Days',
  tokenExpiry90d: '90 Days',
  tokenExpiryPermanent: 'Permanent',
  tokenCreated: 'Created',
  tokenExpires: 'Expires',
  tokenPermanent: 'Never',
  tokenLastUsed: 'Last Used',
  tokenNeverUsed: 'Never',
  tokenReveal: 'Token Generated',
  tokenRevealWarning: 'Copy and save this token now. It will not be shown again.',
  tokenCopyButton: 'Copy',
  tokenCopied: 'Copied!',
  tokenRevoke: 'Revoke',
  tokenRevokeConfirm: 'Revoke this token?',
  tokenGenerate: 'Generate Token',
  tokenNoTokens: 'No tokens yet',

  // Logs
  navLogs: 'Logs',
  logAgentAudit: 'Agent Audit',
  logAccess: 'Access Logs',
  logTokenUsage: 'Token Usage',
  logSystemEvents: 'System Events',
  logTime: 'Time',
  logAgent: 'Agent',
  logTool: 'Tool',
  logStatus: 'Status',
  logReason: 'Reason',
  logUser: 'User',
  logMethod: 'Method',
  logPath: 'Path',
  logLatency: 'Latency(ms)',
  logClientIP: 'Client IP',
  logPromptTokens: 'Prompt',
  logCompletionTokens: 'Completion',
  logTotalTokens: 'Total',
  logStage: 'Stage',
  logEventType: 'Type',
  logDetail: 'Detail',
  logPrevPage: 'Prev',
  logNextPage: 'Next',
  logPageInfo: 'Page {page} / {total} total',
  logNoData: 'No log records',
  logFilterPath: 'Filter by path...',
  logDetailTitle: 'Log Detail',
  logClickHint: 'Click on a row to view full details',
  logFieldId: 'ID',
  logFieldAgentId: 'Agent ID',
  logFieldWorkflowId: 'Workflow',
  logFieldTaskId: 'Task',
  logFieldSessionId: 'Session',
  logFieldParameters: 'Parameters',
  logFieldClientIP: 'Client IP',
  logFieldUserId: 'User ID',
  logFieldAgentProfileId: 'Agent Profile',
  logFieldSource: 'Source',
  logFieldCreatedAt: 'Created At',
  logFieldPromptTokens: 'Prompt Tokens',
  logFieldCompletionTokens: 'Completion Tokens',
  logCopyId: 'Copy ID',

} as const;

