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



  // Agents

  agents: 'Agents',

  noAgents: 'No agents found on this node',

  scanAgents: 'Scan',

  scanning: 'Scanning...',

  maxSessions: 'Max sessions',

  agent: 'Agent',

  selectAgent: 'Select an agent...',

  noAgentsOnNode: 'This node has no available agents. Click "Scan" to detect installed agents.',

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

  nodeStart: 'Start',
  nodeStop: 'Stop',
  nodeStarting: 'Starting...',
  nodeStopping: 'Stopping...',

  alreadyHasNode: 'You already have an active node. Remove it first.',



  // Notifications

  pendingInvitations: 'Pending Invitations',

  invitationFrom: '{name} invited you to {workspace}',

  accept: 'Accept',

  decline: 'Decline',

  noPendingInvitations: 'No pending invitations',

  invitationAccepted: 'Invitation accepted',

  invitationDeclined: 'Invitation declined',

} as const;

