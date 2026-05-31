export const zh = {
  // App
  appTitle: 'Superco',
  appSubtitle: 'AI Agent 分布式调度平台',

  // Auth
  login: '登录',
  register: '注册',
  username: '用户名',
  password: '密码',
  logout: '退出登录',
  alreadyHasAccount: '已有账号？去登录',
  noAccount: '没有账号？去注册',
  authFailed: '认证失败',

  // Sidebar
  navNodes: '节点',
  navSessions: '会话',
  navTerminal: '终端',

  // Nodes
  agentNodes: 'Agent 节点',
  loadingNodes: '加载节点中...',
  noNodes: '暂无注册节点。请先启动 Agent Node。',
  refresh: '刷新',
  lastSeen: '最后在线',
  nodeOnline: '在线',
  nodeOffline: '离线',
  nodeBusy: '忙碌',

  // Sessions
  sessions: '会话',
  loadingSessions: '加载会话中...',
  noSessions: '暂无会话。创建一个开始使用。',
  workspace: '工作区',
  created: '创建于',
  sessionPending: '等待中',
  sessionRunning: '运行中',
  sessionPaused: '已暂停',
  sessionStatusCompleted: '已完成',
  sessionStatusFailed: '失败',

  // Create Session
  newSession: '新建会话',
  targetNode: '目标节点',
  selectNode: '选择一个节点...',
  noOnlineNodes: '没有在线节点',
  workspacePath: '工作区路径',
  workspacePlaceholder: '/home/user/project 或 C:\\Users\\me\\project',
  prompt: '提示词',
  promptPlaceholder: '描述要 Claude Code 执行的任务...',
  allFieldsRequired: '所有字段为必填',
  creating: '创建中...',
  startSession: '启动会话',
  failedToCreate: '创建会话失败',

  // Terminal
  terminal: '终端',
  session: '会话',
  none: '无',
  disconnect: '断开连接',
  noActiveSession: '没有活跃会话。请先在"会话"标签页创建一个会话。',
  waitingForSession: '等待会话中...',
  sessionCompleted: '[会话执行成功]',
  sessionFailed: '[会话执行失败：',
  unknownError: '未知错误',

  // Agents
  agents: 'Agent',
  noAgents: '该节点没有 Agent',
  scanAgents: '扫描',
  scanning: '扫描中...',
  maxSessions: '最大会话数',
  agent: 'Agent',
  selectAgent: '选择一个 Agent...',
  noAgentsOnNode: '该节点没有可用 Agent。点击"扫描"检测已安装的 AI 工具。',
  enabled: '已启用',
  disabled: '已禁用',
  agentHint: 'Agent 从 PATH 中自动检测。可切换启用/禁用。',
  loading: '加载中',
  optional: '可选',

  // Permission Mode
  permissionMode: '权限模式',
  autoMode: '自动模式',
  restrictedMode: '受限模式',

  // Chat
  chat: '聊天',
  noMessages: '暂无消息。发送消息开始对话。',
  send: '发送',
  inputPlaceholder: '输入消息，按 Enter 发送...',
  connecting: '连接中...',
  sessionActive: '活跃',
  clear: '清空',
  sessionNoActive: '无活跃会话',
  connected: '已连接',
  offline: '离线',
  you: '你',
  system: '系统',

  // Language
  switchLang: 'English',
} as const;
