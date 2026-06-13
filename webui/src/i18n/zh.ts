export const zh = {

  // App

  appTitle: 'CoAether',

  appSubtitle: 'AI Agent 分布式调度平台',



  // Auth

  login: '登录',

  register: '注册',

  username: '用户名',

  email: '邮箱',

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

  showOffline: '显示离线节点',
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



  // Projects

  navProjects: '项目',

  projectCreate: '创建项目',

  projectName: '项目名称',

  projectDescription: '项目描述',

  projectColor: '颜色',

  projectEmpty: '还没有项目',

  noProject: '无项目',



  // Tasks

  navTasks: '任务',

  navTrash: '回收站',

  taskCreate: '创建任务',

  taskEdit: '编辑任务',

  taskDelete: '删除',

  taskConfirmDelete: '确定删除该任务？',

  taskStatusTodo: '待办',

  taskStatusInProgress: '进行中',

  taskStatusBlocked: '阻塞',

  taskStatusDone: '完成',

  taskStatusCompleted: '已完成',
  taskStatusStuck: '熔断',
  taskStatusReview: '审核',

  taskTitle: '标题',

  taskDescription: '描述',

  taskStatus: '状态',

  taskEmpty: '还没有任务',

  taskViewKanban: '看板',

  taskViewList: '列表',

  taskProgress: '推进',

  taskActions: '操作',

  taskRestore: '恢复',

  taskPermanentDelete: '永久删除',

  taskTrashEmpty: '回收站为空',
  defaultProject: '默认项目',
  taskFilterPriority: '优先级',
  creator: '创建者',
  taskDelegated: '执行人',
  deleteVerifyPrompt: '请回答以下验证问题：',
  deleteVerifyWrong: '答案错误，请重试',

  // Task Detail
  taskDetailSaving: '保存中...',
  taskDetailUnknown: '未知',
  taskDetailUpdated: '已更新',
  taskDetailNoDescription: '暂无描述',
  taskDetailSubtasks: '子任务',
  taskDetailComments: '评论',
  taskDetailAgentBadge: '智能体',
  taskDetailAgentReport: '智能体报告',
  taskDetailOverdue: '该任务已逾期',
  taskDetailPriority: '优先级',
  priorityUrgent: '紧急',
  priorityHigh: '高',
  priorityMedium: '中',
  priorityLow: '低',
  taskDetailAssignee: '负责人',
  taskDetailUnassigned: '未分配',
  taskDetailDelegatedAssignees: '委托负责人',
  taskDetailRemoveAssignee: '移除负责人',
  taskDetailUser: '用户',
  taskDetailSelect: '请选择...',
  taskDetailAdd: '添加',
  taskDetailAddAssignee: '+ 添加负责人',
  taskDetailAddTag: '添加标签...',
  taskDetailTags: '标签',
  taskDetailDueDate: '截止日期',
  taskDetailProject: '项目',
  taskDetailNoProject: '无项目',
  taskDetailParentTask: '父任务',
  taskDetailNoneTopLevel: '无（顶层任务）',
  taskDetailCreated: '创建于：',
  taskDetailUpdatedTime: '更新于：',
  taskDetailClose: '关闭',
  taskDetailDeleteTask: '删除任务',
  taskDetailConfirmDeleteMsg: '确定要删除该任务吗？此操作不可撤销。',
  taskDetailDeleteCommentHint: '再次点击确认删除',
  taskDetailDeleteCommentTitle: '删除评论',

  // Review
  reviewApprove: '批准',
  reviewReject: '驳回',
  reviewComment: '审核意见',
  reviewCommentPlaceholder: '输入审核意见...',
  reviewApproved: '已批准',
  reviewRejected: '已驳回',
  reviewApproveSuccess: '任务已批准',
  reviewRejectSuccess: '任务已驳回',
  reviewAgentLoopCount: '智能体轮次',
  reviewMaxLoops: '最大轮次',
  reviewMeltdown: '熔断',

  // Workflow
  taskWorkflow: '工作流',
  taskAgentLoops: '智能体循环',
  taskAgentLoopCount: '当前轮次',
  taskMaxAgentLoops: '最大轮次',
  taskCompletionBehavior: '完成行为',
  completionBehaviorAutoDone: '自动完成',
  completionBehaviorAutoReview: '自动审核',
  completionBehaviorSampleReview: '抽检',
  completionBehaviorNeedsReview: '需要人工审核',

  // Agents

  agents: '智能体',

  noAgents: '该节点没有 Agent',

  scanAgents: '扫描',

  scanning: '扫描中...',

  maxSessions: '最大会话数',

  agent: 'Agent',

  selectAgent: '选择一个 Agent...',

  noAgentsOnNode: '该节点没有可用 Agent。点击"扫描"检测已安装的 AI 工具。',
  assignedAgents: '已分配的智能体',
  noAssignedAgents: '没有已分配的智能体',

  enabled: '已启用',

  disabled: '已禁用',

  agentHint: 'Agent 从 PATH 中自动检测。可切换启用/禁用。',

  loading: '加载中',

  optional: '可选',



  // Agent Profiles

  agentProfiles: '智能体配置',

  createAgent: '添加智能体',

  editAgent: '编辑智能体',

  deleteAgent: '删除智能体',

  agentName: '名称',

  agentNamePlaceholder: '给智能体起个名字...',

  agentDescription: '描述',

  agentDescriptionPlaceholder: '描述这个智能体的用途...',

  agentRuntime: '运行时能力',

  selectRuntime: '选择一个运行时能力...',
  agentNode: '运行节点',

  saveAgent: '保存',

  cancel: '取消',

  confirmDelete: '确定删除该智能体？',

  createSuccess: '创建成功',

  updateSuccess: '更新成功',

  deleteSuccess: '删除成功',

  noProfiles: '还没有智能体，点击"+"添加一个吧',

  profileDetail: '详情',

  profileEdit: '编辑',



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



  // Workspaces

  workspaceLabel: '工作区',

  manageWorkspaces: '管理工作区',

  addWorkspace: '添加工作区',

  workspaceName: '工作区名称',

  workspaceDescription: '工作区描述',

  workspaceCreate: '创建工作区',

  workspaceDelete: '删除工作区',

  workspaceDeleteConfirm: '删除该工作区？任务和项目将成为未分配状态。',

  workspaceDefaultName: '默认',



  // Language

  switchLang: 'English',



  // Plugins

  navPlugins: '插件',

  plugins: '插件管理',

  pluginLoading: '加载插件中...',

  noPlugins: '暂未安装插件。',

  pluginRunning: '运行中',

  pluginStopped: '已停止',

  pluginError: '错误',

  pluginStarting: '启动中',

  pluginStopping: '停止中',

  pluginStart: '启动',

  pluginStop: '停止',

  pluginReload: '重载',

  pluginVersion: '版本',

  pluginAuthor: '作者',

  pluginPermissions: '权限',

  pluginHooks: '钩子',

  pluginRoutes: 'API 路由',

  pluginUptime: '运行时间',

  pluginPort: '端口',

  pluginPid: '进程 ID',

  pluginNoPerms: '未声明',

  pluginNoHooks: '无',

  pluginNoRoutes: '无',

  pluginNoError: '无错误',

  pluginConfirmStop: '确定要停止 {name} 吗？',

  pluginConfirmReload: '确定要重载 {name} 吗？',

  pluginInstall: '安装插件',
  pluginInstallUpload: '上传 ZIP',
  pluginInstallGit: 'Git 仓库',
  pluginInstallUploadHint: '选择插件 ZIP 文件进行上传安装。',
  pluginInstallGitHint: '从远程 Git 仓库克隆插件。',
  pluginInstallUrl: 'Git 地址',
  pluginInstallUrlPlaceholder: 'https://github.com/user/plugin.git',
  pluginInstallBranch: '分支（可选）',
  pluginInstallBranchPlaceholder: 'main',
  pluginInstallButton: '安装',
  pluginInstalling: '安装中...',
  pluginInstallSuccess: '插件 {name} v{version} 安装成功',
  pluginInstallError: '安装失败',
  pluginRemove: '删除',
  pluginRemoveConfirm: '确定要删除插件 {name} 吗？',



  // Remote Nodes

  addNode: '添加节点',

  addNodeTitle: '添加远程节点',

  nodeName: '节点名称',

  nodeNamePlaceholder: '例如 my-mac-claude',

  step1: '在目标机器（Mac 或 Windows）上执行以下命令，agent-runtime 将在登录时自动启动。',

  generateCommand: '生成安装命令',

  runOnMac: 'macOS（bash）',

  runOnWindows: 'Windows（PowerShell）',

  copyCommand: '复制',

  copied: '已复制！',

  waitingNode: '等待节点连接...',

  nodeAdded: '节点连接成功！',

  nodeRemove: '移除',

  nodeRemoveConfirm: '确定要移除这个节点吗？',

  nodeStopConfirm: '请完成以下验证以确认停止节点：',
  nodeStopConfirmWrong: '计算结果不正确，请重试',
  nodeNoPermission: '你没有权限操作该节点',
  nodeStart: '启动',
  nodeStop: '停止',
  nodeStarting: '启动中...',
  nodeStopping: '停止中...',
  startCommandTitle: '启动节点',
  startCommandMac: 'macOS / Linux',
  startCommandWindows: 'Windows (PowerShell)',
  startCommandHint: '在目标节点上运行以下命令以启动 Agent Runtime：',

  alreadyHasNode: '你已有一个活跃节点，请先移除它。',



  // Comments
  commentPlaceholder: '输入评论...',
  commentPost: '发布',
  commentEmpty: '暂无评论',
  commentReply: '回复',
  commentDelete: '删除',
  commentDeleteConfirm: '确定删除这条评论？',
  commentBy: '评论于',

  // Notifications

  notifInbox: '通知',
  notifInvitations: '邀请',
  notifEmpty: '暂无通知',
  notifMarkAllRead: '全部标为已读',
  notifTaskAssigned: '任务委派',
  notifTaskStatusChanged: '状态变更',
  notifTaskComment: '新评论',
  notifTimeAgo: '{time}前',

  pendingInvitations: '待处理的邀请',

  invitationFrom: '{name} 邀请你加入 {workspace}',

  accept: '接受',

  decline: '拒绝',

  noPendingInvitations: '暂无待处理的邀请',

  invitationAccepted: '已接受邀请',

  invitationDeclined: '已拒绝邀请',



  // Automation Rules

  ruleName: '规则名称',

  ruleNamePlaceholder: '我的自动化规则',

  ruleDescription: '描述',

  ruleDescriptionPlaceholder: '可选描述',

  ruleTrigger: '触发器',

  ruleConditions: '条件 (JSON)',

  ruleConditionsHint: '格式：field, op (matches/equals/contains), value',

  ruleActions: '动作 (JSON)',

  ruleActionsHint: '数组格式：{type, value} 对象',

  ruleEnabled: '已启用',

  ruleCreate: '创建规则',

  ruleEdit: '编辑规则',

  ruleEmpty: '还没有自动化规则。',

  ruleConfirmDelete: '确定删除该规则？',

  ruleTriggerComment: '评论时',

  ruleTriggerStatus: '状态变更时',

  ruleTriggerAssignee: '负责人变更时',

  ruleTriggerCreate: '创建任务时',

  ruleDisable: '禁用',

  ruleEnable: '启用',

  ruleViewLogs: '查看日志',

  ruleErrorNameRequired: '规则名称不能为空',

  ruleErrorJsonInvalid: '条件或动作的 JSON 格式无效',

  ruleLogTitle: '执行日志',

  ruleLogEmpty: '暂无执行日志。',

  ruleLogTime: '时间',

  ruleLogTask: '任务',

  ruleLogEvent: '事件',

  ruleLogMatched: '匹配',

  ruleLogResult: '结果',

  navAutomation: '规则',

  edit: '编辑',

  delete: '删除',

  save: '保存',

  saving: '保存中...',

  // Skills
  skillLibrary: '技能库',
  skillCreate: '创建',
  skillEdit: '编辑技能',
  skillExtract: '提取',
  skillExtractFromTask: '从任务提取技能',
  skillEmpty: '还没有技能。创建一个或从已完成任务提取。',
  skillNameLabel: '名称',
  skillNamePlaceholder: '技能名称',
  skillDescriptionLabel: '描述',
  skillDescriptionPlaceholder: '可选描述',
  skillContent: '内容',
  skillContentPlaceholder: '技能内容 / 提示词模板...',
  skillUsage: '使用次数',
  skillFromTask: '来自任务',
  skillTaskId: '任务 ID',
  skillTaskIdPlaceholder: '输入任务 ID...',
  navTools: '工具集',
  toolSetTitle: '工具集',
  toolEnabled: '启用',
  toolDisabled: '已禁用',
  toolLinkedAgents: '挂载的智能体',
  toolNoAgents: '暂无智能体挂载此工具',
  toolName_create_sub_task: '创建子任务',
  toolName_propose_decomposition_plan: '提交分解方案',
  toolName_assign_task: '分配任务',
  toolName_review_task: '审核任务',
  toolName_add_comment: '添加评论',
  toolName_get_task_detail: '查看详情',
  toolName_list_sub_tasks: '列出子任务',
  toolName_update_task_status: '更新状态',
  toolName_search_agent_profiles: '搜索智能体',
  navSkills: '技能库',
  navAgentQueue: '队列',

  // Agent Profile Enhancement

  systemPrompt: '系统提示词',

  systemPromptPlaceholder: '定义智能体的角色、性格和专业领域...',

  maxConcurrency: '最大并行任务数',

  abilityTags: '能力标签',

  abilityTagsPlaceholder: '例如：前端、后端、数据库',

  instructions: '行为指令',

  instructionsPlaceholder: '定义智能体与用户沟通的语气、风格和行为准则，使其更具个性...',

  agentLoad: '负载',

  agentLoadInfo: '{current}/{max} 个任务运行中',

  // Agent Harness fields
  reviewSampleRate: '审核采样率',
  // Agent Folders
  agentFolders: '智能体文件夹',
  folderCreate: '新建文件夹',
  folderName: '文件夹名称',
  folderNamePlaceholder: '输入文件夹名称...',
  folderColor: '颜色',
  folderRename: '重命名',
  folderDelete: '删除文件夹',
  folderDeleteConfirm: '确定删除该文件夹？文件夹内的智能体不会被删除。',
  folderUncategorized: '未分类',
  folderAll: '全部智能体',
  folderAddAgent: '添加智能体到文件夹',
  folderRemoveAgent: '从文件夹移除',
  folderEmpty: '还没有文件夹，点击 + 创建一个。',
  folderAgentCount: '{count} 个智能体',

  reviewSampleRateHint: '0.0 = 从不审核, 1.0 = 始终审核',
  reviewTimeout: '审核超时（分钟）',
  maxReviewLoops: '最大审核轮次',
  agentMaxDepth: '最大深度',
  agentSkills: '技能',
  agentProtocolVersion: '协议版本',
  agentCapabilities: '工具集',
  agentPermissions: '权限',

  // Agent Queue
  agentQueue: 'Agent 队列',
  agentQueueEmpty: '暂无排队任务。',
  agentQueueQueued: '排队中',
  agentQueueClaimed: '已认领',
  agentQueueProcessing: '处理中',
  agentQueueCompleted: '已完成',
  agentQueueFailed: '失败',

  // Decomposition Plan
  decompPlanTitle: '分解计划',
  decompPlanPending: '待审核',
  decompPlanApproved: '已通过',
  decompPlanRejected: '已驳回',
  decompPlanBy: '提出人',
  decompPlanSelectAll: '全选',
  decompPlanDeselectAll: '取消全选',
  decompPlanApproveSelected: '批准所选',
  decompPlanApproveAll: '全部批准',
  decompPlanReject: '驳回',
  decompPlanRejectPlaceholder: '输入驳回原因...',
  decompPlanConfirmReject: '确认驳回',
  decompPlanNoItems: '计划中暂无条目。',
  decompPlanSummary: '概要',

  // Token Management
  tokenManagement: 'Token管理',
  tokenList: 'Token列表',
  tokenCreate: '生成Token',
  tokenName: '名称',
  tokenNamePlaceholder: '输入Token名称...',
  tokenExpiry: '有效期',
  tokenExpiry7d: '7天',
  tokenExpiry30d: '30天',
  tokenExpiry90d: '90天',
  tokenExpiryPermanent: '永久',
  tokenCreated: '创建于',
  tokenExpires: '过期时间',
  tokenPermanent: '永不过期',
  tokenLastUsed: '最后使用',
  tokenNeverUsed: '未使用',
  tokenReveal: 'Token已生成',
  tokenRevealWarning: '请立即复制并保存此Token。关闭此窗口后将无法再次查看。',
  tokenCopyButton: '复制',
  tokenCopied: '已复制！',
  tokenRevoke: '撤销',
  tokenRevokeConfirm: '确定要撤销该Token吗？',
  tokenGenerate: '生成Token',
  tokenNoTokens: '暂无Token',

  // Logs
  navLogs: '日志',
  logAgentAudit: 'Agent审计',
  logAccess: '访问日志',
  logTokenUsage: 'Token用量',
  logSystemEvents: '系统事件',
  logTime: '时间',
  logAgent: 'Agent',
  logTool: '工具',
  logStatus: '状态',
  logReason: '原因',
  logUser: '用户',
  logMethod: '方法',
  logPath: '路径',
  logLatency: '耗时(ms)',
  logClientIP: '客户端IP',
  logPromptTokens: '输入',
  logCompletionTokens: '输出',
  logTotalTokens: '总计',
  logStage: '阶段',
  logEventType: '类型',
  logDetail: '详情',
  logPrevPage: '上一页',
  logNextPage: '下一页',
  logPageInfo: '第 {page} 页 / 共 {total} 条',
  logNoData: '暂无日志记录',
  logFilterPath: '按路径筛选...',
  logDetailTitle: '日志详情',
  logClickHint: '点击表格行查看完整详情',
  logFieldId: '编号',
  logFieldAgentId: '智能体ID',
  logFieldWorkflowId: '工作流',
  logFieldTaskId: '任务',
  logFieldSessionId: '会话',
  logFieldParameters: '参数',
  logFieldClientIP: '客户端IP',
  logFieldUserId: '用户ID',
  logFieldAgentProfileId: '智能体配置',
  logFieldSource: '来源',
  logFieldCreatedAt: '创建时间',
  logFieldPromptTokens: '输入Token',
  logFieldCompletionTokens: '输出Token',
  logCopyId: '复制ID',

} as const;

