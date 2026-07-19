const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const https = require('https');
const http = require('http');

// ============================================
// 配置和常量
// ============================================

const SCRIPT_DIR = __dirname;
const ERROR_LOG = path.join(SCRIPT_DIR, 'notify_error.log');
const CONFIG_FILE = path.join(SCRIPT_DIR, 'notify_config.json');
const STYLES_FILE = path.join(SCRIPT_DIR, 'ui_styles.json');

const EVENT_PERMISSION_REQUEST = 'PermissionRequest';
const EVENT_STOP = 'Stop';
const EVENT_PRE_TOOL_USE = 'PreToolUse';
const EVENT_POST_TOOL_USE = 'PostToolUse';
const TOOL_ASK_USER_QUESTION = 'AskUserQuestion';

const SOUND_EXCLAMATION = 'Exclamation';
const SOUND_ASTERISK = 'Asterisk';
const SOUND_QUESTION = 'Question';

function loadConfig() {
  const defaults = {
    notify_url: '',
    sound_enabled: true,
    log_enabled: true
  };

  try {
    if (fs.existsSync(CONFIG_FILE)) {
      const userConfig = JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8'));
      return { ...defaults, ...userConfig };
    }
  } catch (err) {
    // 使用默认配置
  }

  return defaults;
}

const CONFIG = loadConfig();

// ============================================
// UI 样式配置
// ============================================

function loadStyles() {
  const defaults = {
    colors: {
      primary: '#4361ee',
      success: '#10b981',
      danger: '#ef4444',
      background: '#f8f9fa',
      metaBackground: '#e9ecef',
      border: '#dee2e6',
      labelText: '#212529',
      valueText: '#495057',
      descText: '#6c757d',
      white: '#ffffff',
      buttonHover: '#f8f9fa'
    },
    fontSizes: {
      meta: 13,
      title: 13,
      content: 12,
      question: 12,
      optionLabel: 11,
      optionDesc: 10,
      message: 12,
      button: 12,
      customLabel: 9,
      customInput: 10
    },
    spacing: {
      metaPadding: '20,16',
      metaLineSpacing: 2,
      contentPadding: '20',
      titleMargin: '20,18,20,12',
      questionPadding: '20,12,20,12',
      buttonMargin: '0,8,0,0',
      optionMargin: '0,0,0,8',
      optionPadding: '12',
      sectionMargin: '0,0,0,12',
      customInputMargin: '20,12,20,8',
      customLabelMargin: '0,0,0,8',
      separatorMargin: '0,0,0,12'
    },
    window: {
      minWidth: 600,
      maxWidth: 800,
      minHeight: 200,
      maxHeight: 700
    }
  };

  try {
    if (fs.existsSync(STYLES_FILE)) {
      const userStyles = JSON.parse(fs.readFileSync(STYLES_FILE, 'utf8'));
      return {
        colors: { ...defaults.colors, ...userStyles.colors },
        fontSizes: { ...defaults.fontSizes, ...userStyles.fontSizes },
        spacing: { ...defaults.spacing, ...userStyles.spacing },
        window: { ...defaults.window, ...userStyles.window }
      };
    }
  } catch (err) {
    logError(`[Styles] Load error: ${err.message}\n`);
  }

  return defaults;
}

const UI_STYLES = loadStyles();

// ============================================
// 工具函数
// ============================================

function logError(message) {
  if (CONFIG.log_enabled) {
    fs.appendFileSync(ERROR_LOG, message);
  }
}

function extractField(data, fieldName, defaultValue = '') {
  const standardMappings = {
    session_id: ['session_id', 'sessionId'],
    cwd: ['cwd', 'working_directory'],
    transcript_path: ['transcript_path', 'transcriptPath'],
    tool_name: ['tool_name', 'toolName'],
    tool_input: ['tool_input', 'toolInput']
  };

  const fallbacks = standardMappings[fieldName] || [fieldName];

  for (const key of fallbacks) {
    const value = data[key];
    if (value !== undefined && value !== null && value !== '') {
      return value;
    }
  }

  return defaultValue;
}

function encodeUtf8ToBase64(str) {
  return Buffer.from(str, 'utf8').toString('base64');
}

// 生成样式辅助函数
function getStyleHelpers() {
  const { colors, fontSizes, spacing } = UI_STYLES;

  return {
    // 窗口基础样式
    window: `
$window.MinWidth = ${UI_STYLES.window.minWidth}
$window.MaxWidth = ${UI_STYLES.window.maxWidth}
$window.MinHeight = ${UI_STYLES.window.minHeight}
$window.MaxHeight = ${UI_STYLES.window.maxHeight}
$window.WindowStartupLocation = 'CenterScreen'
$window.Topmost = $$true
$window.Background = '${colors.background}'`,

    // 按钮样式（主按钮）
    primaryButton: (varName) => `
${varName}.FontSize = ${fontSizes.button}
${varName}.Padding = '12,8'
${varName}.Margin = '${spacing.buttonMargin}'
${varName}.Background = '${colors.primary}'
${varName}.Foreground = '${colors.white}'
${varName}.BorderThickness = '0'
${varName}.Cursor = 'Hand'
${varName}.add_MouseEnter({
  ${varName}.Opacity = 0.9
})
${varName}.add_MouseLeave({
  ${varName}.Opacity = 1.0
})`,

    // 按钮样式（次要按钮）
    secondaryButton: (varName) => `
${varName}.FontSize = ${fontSizes.button}
${varName}.Padding = '12,8'
${varName}.Margin = '${spacing.buttonMargin}'
${varName}.Background = '${colors.white}'
${varName}.Foreground = '${colors.labelText}'
${varName}.BorderBrush = '${colors.border}'
${varName}.BorderThickness = '1'
${varName}.Cursor = 'Hand'
${varName}.add_MouseEnter({
  ${varName}.Background = '${colors.buttonHover}'
})
${varName}.add_MouseLeave({
  ${varName}.Background = '${colors.white}'
})`,

    // 内容区域样式
    contentPadding: spacing.contentPadding,
    contentFontSize: fontSizes.content,
    messageFontSize: fontSizes.message,

    // 颜色快速访问
    colors,
    fontSizes,
    spacing
  };
}

// 生成统一的元信息栏 PowerShell 代码
function generateMetaInfoBar(sessionId, cwd, additionalFields = []) {
  const projectName = cwd ? path.basename(cwd) : 'Unknown';
  const sessionShort = sessionId ? sessionId.substring(0, 8) : 'Unknown';

  const projectLabel_b64 = encodeUtf8ToBase64('📦 项目');
  const sessionLabel_b64 = encodeUtf8ToBase64('🔖 会话');
  const pathLabel_b64 = encodeUtf8ToBase64('📁 路径');

  const { colors, fontSizes, spacing } = UI_STYLES;

  const additionalRows = additionalFields.map(field => {
    const label_b64 = encodeUtf8ToBase64(field.label);
    const value = field.value.replace(/\\/g, '\\\\').replace(/'/g, "''");
    return `
$${field.id}Row = New-Object System.Windows.Controls.StackPanel
$${field.id}Row.Orientation = 'Horizontal'
$${field.id}Row.Margin = '0,0,0,${spacing.metaLineSpacing}'
$${field.id}Lbl = New-Object System.Windows.Controls.TextBlock
$${field.id}Lbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${label_b64}')) + ': '
$${field.id}Lbl.FontSize = ${fontSizes.meta}; $${field.id}Lbl.FontWeight = 'Bold'; $${field.id}Lbl.Foreground = '${colors.labelText}'
$${field.id}Val = New-Object System.Windows.Controls.TextBlock
$${field.id}Val.Text = '${value}'
$${field.id}Val.FontSize = ${fontSizes.meta}; $${field.id}Val.Foreground = '${colors.valueText}'; $${field.id}Val.FontFamily = 'Consolas'
${field.wrap ? `$${field.id}Val.TextWrapping = 'Wrap'` : ''}
$${field.id}Row.AddChild($${field.id}Lbl); $${field.id}Row.AddChild($${field.id}Val)
$metaPanel.AddChild($${field.id}Row)
`;
  }).join('\n');

  return `
# Meta info bar
$metaBar = New-Object System.Windows.Controls.Border
$metaBar.Background = '${colors.metaBackground}'
$metaBar.Padding = '${spacing.metaPadding}'
$metaBar.BorderBrush = '${colors.border}'
$metaBar.BorderThickness = '0,1,0,1'
[System.Windows.Controls.Grid]::SetRow($metaBar, 1)

$metaPanel = New-Object System.Windows.Controls.StackPanel

$projectRow = New-Object System.Windows.Controls.StackPanel
$projectRow.Orientation = 'Horizontal'
$projectRow.Margin = '0,0,0,${spacing.metaLineSpacing}'
$projectLbl = New-Object System.Windows.Controls.TextBlock
$projectLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${projectLabel_b64}')) + ': '
$projectLbl.FontSize = ${fontSizes.meta}; $projectLbl.FontWeight = 'Bold'; $projectLbl.Foreground = '${colors.labelText}'
$projectVal = New-Object System.Windows.Controls.TextBlock
$projectVal.Text = '${projectName.replace(/'/g, "''")}'
$projectVal.FontSize = ${fontSizes.meta}; $projectVal.Foreground = '${colors.valueText}'; $projectVal.FontFamily = 'Consolas'
$projectRow.AddChild($projectLbl); $projectRow.AddChild($projectVal)
$metaPanel.AddChild($projectRow)

$sessionRow = New-Object System.Windows.Controls.StackPanel
$sessionRow.Orientation = 'Horizontal'
$sessionRow.Margin = '0,0,0,${spacing.metaLineSpacing}'
$sessionLbl = New-Object System.Windows.Controls.TextBlock
$sessionLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${sessionLabel_b64}')) + ': '
$sessionLbl.FontSize = ${fontSizes.meta}; $sessionLbl.FontWeight = 'Bold'; $sessionLbl.Foreground = '${colors.labelText}'
$sessionVal = New-Object System.Windows.Controls.TextBlock
$sessionVal.Text = '${sessionShort}'
$sessionVal.FontSize = ${fontSizes.meta}; $sessionVal.Foreground = '${colors.valueText}'; $sessionVal.FontFamily = 'Consolas'
$sessionRow.AddChild($sessionLbl); $sessionRow.AddChild($sessionVal)
$metaPanel.AddChild($sessionRow)

$pathRow = New-Object System.Windows.Controls.StackPanel
$pathRow.Orientation = 'Horizontal'
$pathRow.Margin = '0,0,0,${spacing.metaLineSpacing}'
$pathLbl = New-Object System.Windows.Controls.TextBlock
$pathLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${pathLabel_b64}')) + ': '
$pathLbl.FontSize = ${fontSizes.meta}; $pathLbl.FontWeight = 'Bold'; $pathLbl.Foreground = '${colors.labelText}'
$pathVal = New-Object System.Windows.Controls.TextBlock
$pathVal.Text = '${cwd.replace(/\\/g, '\\\\').replace(/'/g, "''")}'
$pathVal.FontSize = ${fontSizes.meta}; $pathVal.Foreground = '${colors.valueText}'; $pathVal.FontFamily = 'Consolas'
$pathVal.TextWrapping = 'Wrap'
$pathRow.AddChild($pathLbl); $pathRow.AddChild($pathVal)
$metaPanel.AddChild($pathRow)

${additionalRows}

$metaBar.Child = $metaPanel
$grid.AddChild($metaBar)
`;
}

function executePowerShellScript(scriptContent) {
  const tempFile = path.join(SCRIPT_DIR, `temp_ps_${Date.now()}.ps1`);

  try {
    // 写入 UTF-8 BOM 以确保 PowerShell 正确识别编码
    const utf8BOM = Buffer.from([0xEF, 0xBB, 0xBF]);
    const scriptBuffer = Buffer.from(scriptContent, 'utf8');
    const fileBuffer = Buffer.concat([utf8BOM, scriptBuffer]);
    fs.writeFileSync(tempFile, fileBuffer);

    // 执行 PowerShell 脚本
    const result = execSync(`powershell -NoProfile -ExecutionPolicy Bypass -File "${tempFile}"`, {
      encoding: 'utf8',
      timeout: 300000,
      windowsHide: false
    });
    return result.trim();
  } finally {
    try {
      if (fs.existsSync(tempFile)) {
        fs.unlinkSync(tempFile);
      }
    } catch (err) {
      // 忽略清理错误
    }
  }
}

function playSound(soundType) {
  if (!CONFIG.sound_enabled) return;

  try {
    execSync(`powershell -Command "[System.Media.SystemSounds]::${soundType}.Play()"`, {
      windowsHide: true
    });
  } catch (err) {
    // 忽略错误
  }
}

function sendHttpNotification(data) {
  const notifyUrl = (CONFIG.notify_url || '').trim();
  if (!notifyUrl) return;

  try {
    const sessionId = extractField(data, 'session_id', '');
    const cwd = extractField(data, 'cwd', '');
    const projectName = cwd ? path.basename(cwd) : '';
    const sessionShort = sessionId ? sessionId.substring(0, 8) : '';

    const msgParts = ['🤖 Claude 已完成任务'];
    if (projectName) msgParts.push(`📁 项目: ${projectName}`);
    if (sessionShort) msgParts.push(`🔑 会话: ${sessionShort}`);
    if (cwd) msgParts.push(`📂 目录: ${cwd}`);

    const msg = msgParts.join('\n');
    const urlObj = new URL(notifyUrl);
    const isHttps = urlObj.protocol === 'https:';
    const lib = isHttps ? https : http;

    const options = {
      hostname: urlObj.hostname,
      port: urlObj.port || (isHttps ? 443 : 80),
      path: urlObj.pathname + urlObj.search,
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
        'Content-Length': Buffer.byteLength(msg)
      },
      timeout: 2000
    };

    const req = lib.request(options, (res) => {
      res.on('data', () => {});
    });

    req.on('error', () => {});
    req.on('timeout', () => req.destroy());
    req.write(msg);
    req.end();
  } catch (err) {
    // 忽略错误
  }
}

// ============================================
// PermissionRequest 处理
// ============================================

function handlePermissionRequest(data) {
  const toolName = extractField(data, 'tool_name', 'Unknown');

  // AskUserQuestion：不返回决策，让 Claude Code 走默认流显示原生面板。
  // 不可返回 decision.behavior:'allow'——对 AskUserQuestion 而言 allow-alone
  // 不充分（官方明文），会导致"放行了无答案的提问"而静默未作答。
  // 正常路径下 PreToolUse 已用 allow+updatedInput.answers 接管、不会走到这里；
  // 走到这里说明用户点了"知道了"或弹窗出错，此时回退原生面板。
  if (toolName === TOOL_ASK_USER_QUESTION) {
    return;
  }

  playSound(SOUND_EXCLAMATION);

  const toolInput = extractField(data, 'tool_input', {});
  const suggestions = data.permission_suggestions || [];
  const cwd = extractField(data, 'cwd', '');
  const sessionId = extractField(data, 'session_id', '');

  const details = Object.entries(toolInput)
    .filter(([k]) => k !== 'description')
    .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : v}`)
    .join('\n');

  const title_b64 = encodeUtf8ToBase64('Claude Code · 权限申请');
  const needAuth_b64 = encodeUtf8ToBase64('需要授权执行操作');
  const toolLabel_b64 = encodeUtf8ToBase64('工具');
  const detailsLabel_b64 = encodeUtf8ToBase64('参数详情');
  const allowBtn_b64 = encodeUtf8ToBase64('同意');
  const rememberBtn_b64 = encodeUtf8ToBase64('同意并记住');
  const denyBtn_b64 = encodeUtf8ToBase64('拒绝');

  const psScript = `
# 强制使用 UTF-8 编码
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$PSDefaultParameterValues['*:Encoding'] = 'utf8'
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

[System.Media.SystemSounds]::Exclamation.Play()

$w = New-Object System.Windows.Window
$w.Title = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${title_b64}'))
$w.Width = 700
$w.Height = 450
$w.WindowStartupLocation = 'CenterScreen'
$w.ResizeMode = 'NoResize'
$w.Background = '#f8f9fa'
$w.Topmost = $true

$w.Add_Loaded({ $w.Activate(); $w.Focus() })

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '0'

$row0 = New-Object System.Windows.Controls.RowDefinition; $row0.Height = 'Auto'
$row1 = New-Object System.Windows.Controls.RowDefinition; $row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition; $row2.Height = '*'
$row3 = New-Object System.Windows.Controls.RowDefinition; $row3.Height = 'Auto'
$grid.RowDefinitions.Add($row0); $grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2); $grid.RowDefinitions.Add($row3)

# Title
$titlePanel = New-Object System.Windows.Controls.StackPanel
$titlePanel.Margin = '20,18,20,12'
[System.Windows.Controls.Grid]::SetRow($titlePanel, 0)

$titleText = New-Object System.Windows.Controls.TextBlock
$titleText.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${needAuth_b64}'))
$titleText.FontSize = 13; $titleText.FontWeight = 'Bold'; $titleText.Foreground = '#212529'
$titlePanel.AddChild($titleText)
$grid.AddChild($titlePanel)

${generateMetaInfoBar(sessionId, cwd)}

# Content
$contentFrame = New-Object System.Windows.Controls.Border
$contentFrame.Margin = '16,16,16,2'
[System.Windows.Controls.Grid]::SetRow($contentFrame, 2)

$scrollViewer = New-Object System.Windows.Controls.ScrollViewer
$scrollViewer.VerticalScrollBarVisibility = 'Auto'
$scrollViewer.HorizontalScrollBarVisibility = 'Disabled'

$card = New-Object System.Windows.Controls.StackPanel
$card.Background = '#ffffff'
$card.Margin = '0'
$card.Padding = '16'

$toolLbl = New-Object System.Windows.Controls.TextBlock
$toolLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${toolLabel_b64}'))
$toolLbl.FontSize = 12; $toolLbl.FontWeight = 'Bold'; $toolLbl.Foreground = '#212529'
$card.AddChild($toolLbl)

$toolBlock = New-Object System.Windows.Controls.TextBlock
$toolBlock.Text = "${toolName}"
$toolBlock.FontSize = 12; $toolBlock.FontWeight = 'Bold'
$toolBlock.Foreground = '#495057'; $toolBlock.Margin = '0,4,0,12'
$toolBlock.FontFamily = 'Consolas'
$card.AddChild($toolBlock)

$detailsLbl = New-Object System.Windows.Controls.TextBlock
$detailsLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${detailsLabel_b64}'))
$detailsLbl.FontSize = 12; $detailsLbl.FontWeight = 'Bold'; $detailsLbl.Foreground = '#212529'
$card.AddChild($detailsLbl)

$detailsBlock = New-Object System.Windows.Controls.TextBlock
$detailsBlock.Text = @'
${details}
'@
$detailsBlock.FontSize = 12; $detailsBlock.Foreground = '#495057'
$detailsBlock.TextWrapping = 'Wrap'; $detailsBlock.FontFamily = 'Consolas'
$detailsBlock.Margin = '0,4,0,0'
$card.AddChild($detailsBlock)

$scrollViewer.Content = $card
$contentFrame.Child = $scrollViewer
$grid.AddChild($contentFrame)

# Buttons
$btnPanel = New-Object System.Windows.Controls.StackPanel
$btnPanel.Orientation = 'Horizontal'
$btnPanel.HorizontalAlignment = 'Right'
$btnPanel.Margin = '16,12,16,16'
[System.Windows.Controls.Grid]::SetRow($btnPanel, 3)

$denyBtn = New-Object System.Windows.Controls.Button
$denyBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${denyBtn_b64}'))
$denyBtn.Padding = '20,8'; $denyBtn.FontSize = 10; $denyBtn.FontWeight = 'Bold'
$denyBtn.Background = '#ef4444'; $denyBtn.Foreground = '#ffffff'
$denyBtn.BorderThickness = 0; $denyBtn.Cursor = 'Hand'
$denyBtn.Add_Click({ $w.Tag = 'deny'; $w.DialogResult = $true; $w.Close() })
$btnPanel.AddChild($denyBtn)

$allowBtn = New-Object System.Windows.Controls.Button
$allowBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${allowBtn_b64}'))
$allowBtn.Padding = '20,8'; $allowBtn.FontSize = 10; $allowBtn.Margin = '8,0,0,0'
$allowBtn.FontWeight = 'Bold'
$allowBtn.Background = '#4361ee'; $allowBtn.Foreground = '#ffffff'
$allowBtn.BorderThickness = 0; $allowBtn.Cursor = 'Hand'
$allowBtn.Add_Click({ $w.Tag = 'allow'; $w.DialogResult = $true; $w.Close() })
$btnPanel.AddChild($allowBtn)

$rememberBtn = New-Object System.Windows.Controls.Button
$rememberBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${rememberBtn_b64}'))
$rememberBtn.Padding = '20,8'; $rememberBtn.FontSize = 10; $rememberBtn.Margin = '8,0,0,0'
$rememberBtn.FontWeight = 'Bold'
$rememberBtn.Background = '#10b981'; $rememberBtn.Foreground = '#ffffff'
$rememberBtn.BorderThickness = 0; $rememberBtn.Cursor = 'Hand'
$rememberBtn.Add_Click({ $w.Tag = 'remember'; $w.DialogResult = $true; $w.Close() })
$btnPanel.AddChild($rememberBtn)

$grid.AddChild($btnPanel)

$w.Content = $grid
$null = $w.ShowDialog()
Write-Output $w.Tag
`;

  try {
    const result = executePowerShellScript(psScript);

    const output = {
      hookSpecificOutput: {
        hookEventName: 'PermissionRequest',
        decision: {}
      }
    };

    if (result === 'allow') {
      output.hookSpecificOutput.decision.behavior = 'allow';
    } else if (result === 'remember') {
      output.hookSpecificOutput.decision.behavior = 'allow';
      const sugg = suggestions[0] || {
        type: 'addRules',
        rules: [{ toolName }],
        behavior: 'allow',
        destination: 'projectSettings'
      };
      sugg.destination = 'projectSettings';
      output.hookSpecificOutput.decision.updatedPermissions = [sugg];
    } else {
      output.hookSpecificOutput.decision.behavior = 'deny';
    }

    console.log(JSON.stringify(output));
  } catch (err) {
    logError(`[PermissionRequest] Error: ${err.message}\n`);
  }
}

// ============================================
// PreToolUse 处理 (AskUserQuestion)
// ============================================

function handlePreToolUse(data) {
  const toolName = extractField(data, 'tool_name');

  if (toolName !== TOOL_ASK_USER_QUESTION) {
    return;
  }

  const toolInput = extractField(data, 'tool_input', {});
  const questions = toolInput.questions || [];
  const cwd = extractField(data, 'cwd', '');
  const sessionId = extractField(data, 'session_id', '');

  if (questions.length === 0) {
    return;
  }

  const q = questions[0];
  const options = q.options || [];
  const multiSelect = q.multiSelect === true;

  const modeLabel_b64 = encodeUtf8ToBase64('类型');
  const modeValue_b64 = encodeUtf8ToBase64(multiSelect ? '多选' : '单选');
  const header_b64 = encodeUtf8ToBase64(q.header || '问题');
  const question_b64 = encodeUtf8ToBase64(q.question || '');

  const additionalFields = [
    { id: 'qtype', label: '📝 类型', value: multiSelect ? '多选' : '单选', wrap: false }
  ];

  const optionButtons = options
    .map((opt, idx) => {
      const label_b64 = encodeUtf8ToBase64(opt.label || `选项 ${idx + 1}`);
      const desc_b64 = encodeUtf8ToBase64(opt.description || '');
      const labelEscaped = (opt.label || '').replace(/'/g, "''");

      // 选项内容（label + desc），单选 Button / 多选 CheckBox 共用
      const contentCode = `
$btnContent${idx} = New-Object System.Windows.Controls.StackPanel
$btnLabel${idx} = New-Object System.Windows.Controls.TextBlock
$btnLabel${idx}.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${label_b64}'))
$btnLabel${idx}.FontWeight = 'Bold'; $btnLabel${idx}.FontSize = 12; $btnLabel${idx}.Foreground = '#212529'
$btnContent${idx}.AddChild($btnLabel${idx})
$btnDesc${idx} = New-Object System.Windows.Controls.TextBlock
$btnDesc${idx}.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${desc_b64}'))
$btnDesc${idx}.FontSize = 12; $btnDesc${idx}.Foreground = '#6c757d'; $btnDesc${idx}.Margin = '0,3,0,0'; $btnDesc${idx}.TextWrapping = 'Wrap'
$btnContent${idx}.AddChild($btnDesc${idx})
`;

      if (multiSelect) {
        // 多选：CheckBox，勾选后由"提交多选答案"统一收集，不立即关窗
        return `
$btn${idx} = New-Object System.Windows.Controls.CheckBox
$btn${idx}.Tag = '${labelEscaped}'
$btn${idx}.Margin = '0,0,0,4'; $btn${idx}.Padding = '8'
$btn${idx}.Background = '#ffffff'; $btn${idx}.BorderBrush = '#dee2e6'; $btn${idx}.BorderThickness = '1'
$btn${idx}.Cursor = 'Hand'
${contentCode}
$btn${idx}.Content = $btnContent${idx}
$optionsPanel.AddChild($btn${idx})
`;
      }

      // 单选：Button，点击即提交并关窗
      return `
$btn${idx} = New-Object System.Windows.Controls.Button
$btn${idx}.Tag = '${labelEscaped}'
$btn${idx}.Margin = '0,0,0,4'; $btn${idx}.Padding = '8'
$btn${idx}.Background = '#ffffff'; $btn${idx}.BorderBrush = '#dee2e6'; $btn${idx}.BorderThickness = '1'
$btn${idx}.Cursor = 'Hand'; $btn${idx}.HorizontalContentAlignment = 'Left'
${contentCode}
$btn${idx}.Content = $btnContent${idx}
$btn${idx}.Add_Click({ $w.Tag = $this.Tag; $w.DialogResult = $true; $w.Close() })
$btn${idx}.Add_MouseEnter({ $this.Background = '#f8f9fa' })
$btn${idx}.Add_MouseLeave({ $this.Background = '#ffffff' })
$optionsPanel.AddChild($btn${idx})
`;
    })
    .join('\n');

  const customInputLabel_b64 = encodeUtf8ToBase64(multiSelect ? '补充自定义答案（与勾选项一起提交）:' : '或输入自定义答案:');
  const submitCustom_b64 = encodeUtf8ToBase64('提交自定义答案');
  const submitMulti_b64 = encodeUtf8ToBase64('提交多选答案');
  const knowBtn_b64 = encodeUtf8ToBase64('知道了（回到 CLI）');

  const psScript = `
# 强制使用 UTF-8 编码
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$PSDefaultParameterValues['*:Encoding'] = 'utf8'
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

[System.Media.SystemSounds]::Question.Play()

$w = New-Object System.Windows.Window
$w.Title = 'Claude Code · 问题'
$w.Width = 700
$w.Height = 650
$w.WindowStartupLocation = 'CenterScreen'
$w.ResizeMode = 'NoResize'
$w.Background = '#f8f9fa'
$w.Tag = ''
$w.Topmost = $true

$w.Add_Loaded({ $w.Activate(); $w.Focus() })

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '0'

$row0 = New-Object System.Windows.Controls.RowDefinition; $row0.Height = 'Auto'
$row1 = New-Object System.Windows.Controls.RowDefinition; $row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition; $row2.Height = 'Auto'
$row3 = New-Object System.Windows.Controls.RowDefinition; $row3.Height = '*'
$row4 = New-Object System.Windows.Controls.RowDefinition; $row4.Height = 'Auto'
$row5 = New-Object System.Windows.Controls.RowDefinition; $row5.Height = 'Auto'
$grid.RowDefinitions.Add($row0); $grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2); $grid.RowDefinitions.Add($row3)
$grid.RowDefinitions.Add($row4); $grid.RowDefinitions.Add($row5)

# Title
$titlePanel = New-Object System.Windows.Controls.StackPanel
$titlePanel.Margin = '20,18,20,12'
[System.Windows.Controls.Grid]::SetRow($titlePanel, 0)

$titleBlock = New-Object System.Windows.Controls.TextBlock
$titleBlock.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${header_b64}'))
$titleBlock.FontSize = 13
$titleBlock.FontWeight = 'Bold'
$titleBlock.Foreground = '#212529'
$titlePanel.AddChild($titleBlock)
$grid.AddChild($titlePanel)

${generateMetaInfoBar(sessionId, cwd, additionalFields)}

# Question
$questionPanel = New-Object System.Windows.Controls.Border
$questionPanel.Padding = '20,12,20,12'
[System.Windows.Controls.Grid]::SetRow($questionPanel, 2)

$questionBlock = New-Object System.Windows.Controls.TextBlock
$questionBlock.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${question_b64}'))
$questionBlock.FontSize = 12
$questionBlock.FontWeight = 'Bold'
$questionBlock.Foreground = '#212529'
$questionBlock.TextWrapping = 'Wrap'
$questionPanel.Child = $questionBlock
$grid.AddChild($questionPanel)

# Options
$scrollViewer = New-Object System.Windows.Controls.ScrollViewer
$scrollViewer.VerticalScrollBarVisibility = 'Auto'
$scrollViewer.HorizontalScrollBarVisibility = 'Disabled'
$scrollViewer.Margin = '20,0,20,0'
[System.Windows.Controls.Grid]::SetRow($scrollViewer, 3)

$optionsPanel = New-Object System.Windows.Controls.StackPanel
${optionButtons}

$scrollViewer.Content = $optionsPanel
$grid.AddChild($scrollViewer)

# Custom input section
$customInputPanel = New-Object System.Windows.Controls.StackPanel
$customInputPanel.Margin = '20,12,20,8'
[System.Windows.Controls.Grid]::SetRow($customInputPanel, 4)

$separator2 = New-Object System.Windows.Controls.Border
$separator2.Height = 1
$separator2.Background = '#dee2e6'
$separator2.Margin = '0,0,0,12'
$customInputPanel.AddChild($separator2)

$customLabel = New-Object System.Windows.Controls.TextBlock
$customLabel.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${customInputLabel_b64}'))
$customLabel.FontSize = 12
$customLabel.FontWeight = 'Bold'
$customLabel.Foreground = '#212529'
$customLabel.Margin = '0,0,0,8'
$customInputPanel.AddChild($customLabel)

$customInput = New-Object System.Windows.Controls.TextBox
$customInput.Padding = '8'
$customInput.FontSize = 12
$customInput.Background = '#ffffff'
$customInput.BorderBrush = '#dee2e6'
$customInputPanel.AddChild($customInput)

$grid.AddChild($customInputPanel)

# Bottom buttons
$btnPanel = New-Object System.Windows.Controls.StackPanel
$btnPanel.Orientation = 'Horizontal'
$btnPanel.HorizontalAlignment = 'Right'
$btnPanel.Margin = '20,12,20,16'
[System.Windows.Controls.Grid]::SetRow($btnPanel, 5)

$knowBtn = New-Object System.Windows.Controls.Button
$knowBtn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${knowBtn_b64}'))
$knowBtn.Padding = '16,8'
$knowBtn.FontSize = 10
$knowBtn.FontWeight = 'Bold'
$knowBtn.Background = '#6c757d'
$knowBtn.Foreground = '#ffffff'
$knowBtn.BorderThickness = 0
$knowBtn.Cursor = 'Hand'
$knowBtn.Add_Click({ $w.Tag = ''; $w.DialogResult = $false; $w.Close() })
$btnPanel.AddChild($knowBtn)

${multiSelect ? `
$submitMulti = New-Object System.Windows.Controls.Button
$submitMulti.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${submitMulti_b64}'))
$submitMulti.Padding = '16,8'; $submitMulti.FontSize = 10; $submitMulti.FontWeight = 'Bold'; $submitMulti.Margin = '8,0,0,0'
$submitMulti.Background = '#4361ee'; $submitMulti.Foreground = '#ffffff'; $submitMulti.BorderThickness = 0; $submitMulti.Cursor = 'Hand'
$submitMulti.Add_Click({
  $selected = @()
  foreach ($child in $optionsPanel.Children) {
    if ($child -is [System.Windows.Controls.CheckBox] -and $child.IsChecked -eq $true) {
      $selected += $child.Tag
    }
  }
  $customText = $customInput.Text.Trim()
  if ($customText -ne '') { $selected += $customText }
  if ($selected.Count -gt 0) {
    $json = $selected | ConvertTo-Json -Compress
    if ($selected.Count -eq 1) { $json = '[' + $json + ']' }
    $w.Tag = $json
    $w.DialogResult = $true
    $w.Close()
  }
})
$btnPanel.AddChild($submitMulti)
` : ''}

${!multiSelect ? `
$submitCustom = New-Object System.Windows.Controls.Button
$submitCustom.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${submitCustom_b64}'))
$submitCustom.Padding = '16,8'
$submitCustom.FontSize = 10
$submitCustom.FontWeight = 'Bold'
$submitCustom.Margin = '8,0,0,0'
$submitCustom.Background = '#10b981'
$submitCustom.Foreground = '#ffffff'
$submitCustom.BorderThickness = 0
$submitCustom.Cursor = 'Hand'
$submitCustom.Add_Click({
  $text = $customInput.Text.Trim()
  if ($text -ne '') {
    $w.Tag = $text
    $w.DialogResult = $true
    $w.Close()
  }
})
$btnPanel.AddChild($submitCustom)
` : ''}

$grid.AddChild($btnPanel)

$w.Content = $grid
$null = $w.ShowDialog()
Write-Output $w.Tag
`;

  try {
    const selectedOption = executePowerShellScript(psScript);

    if (selectedOption) {
      const answers = {};
      if (multiSelect) {
        // 多选：PowerShell 返回 JSON 数组字符串（如 ["a","b"]），parse 为 label 数组
        let selectedArr;
        try {
          selectedArr = JSON.parse(selectedOption);
          if (!Array.isArray(selectedArr)) selectedArr = [String(selectedArr)];
        } catch (err) {
          selectedArr = [selectedOption];
        }
        answers[q.question] = selectedArr;
      } else {
        answers[q.question] = selectedOption;
      }

      const output = {
        hookSpecificOutput: {
          hookEventName: 'PreToolUse',
          permissionDecision: 'allow',
          updatedInput: {
            ...toolInput,
            answers: answers
          }
        }
      };

      console.log(JSON.stringify(output));
    } else {
      // "知道了（回到 CLI）"路径：不返回任何决策（exit 0 无 JSON），
      // 让 Claude Code 走默认权限流、显示原生 AskUserQuestion 面板，
      // 以便用户回 CLI 查看上下文后在原生面板作答。
      // 不可返回 permissionDecision:'defer'——该值仅 claude -p 非交互模式生效，
      // 交互式 CLI 下会被忽略（官方 Hooks reference 明文），导致面板自动取消。
    }
  } catch (err) {
    logError(`[PreToolUse] Error: ${err.message}\n`);
    // 弹窗异常时不返回决策，走默认流显示原生面板（原 defer 在交互式 CLI 下无效）。
  }
}

// ============================================
// Stop 事件处理
// ============================================

function handleStop(data) {
  playSound(SOUND_ASTERISK);
  sendHttpNotification(data);

  const cwd = extractField(data, 'cwd', '');
  const sessionId = extractField(data, 'session_id', '');
  const effortLevel = data.effort?.level || '';
  const permissionMode = data.permission_mode || '';
  const lastMessage = data.last_assistant_message || '';
  const backgroundTasks = (data.background_tasks || []).length;

  const additionalFields = [];
  if (effortLevel) {
    additionalFields.push({ id: 'effort', label: '🧠 思考级别', value: effortLevel, wrap: false });
  }
  if (permissionMode) {
    additionalFields.push({ id: 'permission', label: '🔐 权限模式', value: permissionMode, wrap: false });
  }
  if (backgroundTasks > 0) {
    additionalFields.push({ id: 'tasks', label: '⏳ 后台任务', value: `${backgroundTasks} 个`, wrap: false });
  }

  const title_b64 = encodeUtf8ToBase64('Claude Code · 任务完成');
  const heading_b64 = encodeUtf8ToBase64('Claude 已完成，等待输入');
  const msgLabel_b64 = encodeUtf8ToBase64('最后消息');
  const btnText_b64 = encodeUtf8ToBase64('知道了');

  const psScript = `
# 强制使用 UTF-8 编码
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$PSDefaultParameterValues['*:Encoding'] = 'utf8'
$OutputEncoding = [System.Text.Encoding]::UTF8

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore

$w = New-Object System.Windows.Window
$w.Title = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${title_b64}'))
$w.Width = 600
$w.Height = ${lastMessage ? 450 : 350}
$w.WindowStartupLocation = 'CenterScreen'
$w.ResizeMode = 'NoResize'
$w.Background = '#f8f9fa'

$grid = New-Object System.Windows.Controls.Grid
$grid.Margin = '0'

$row0 = New-Object System.Windows.Controls.RowDefinition; $row0.Height = 'Auto'
$row1 = New-Object System.Windows.Controls.RowDefinition; $row1.Height = 'Auto'
$row2 = New-Object System.Windows.Controls.RowDefinition; $row2.Height = '*'
$row3 = New-Object System.Windows.Controls.RowDefinition; $row3.Height = 'Auto'
$grid.RowDefinitions.Add($row0); $grid.RowDefinitions.Add($row1)
$grid.RowDefinitions.Add($row2); $grid.RowDefinitions.Add($row3)

# Title
$titlePanel = New-Object System.Windows.Controls.StackPanel
$titlePanel.Margin = '20,18,20,12'
[System.Windows.Controls.Grid]::SetRow($titlePanel, 0)

$titleText = New-Object System.Windows.Controls.TextBlock
$titleText.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${heading_b64}'))
$titleText.FontSize = 13
$titleText.FontWeight = 'Bold'
$titleText.Foreground = '#212529'
$titleText.HorizontalAlignment = 'Center'
$titlePanel.AddChild($titleText)
$grid.AddChild($titlePanel)

${generateMetaInfoBar(sessionId, cwd, additionalFields)}

# Content
${lastMessage ? `
$contentFrame = New-Object System.Windows.Controls.Border
$contentFrame.Margin = '16,16,16,2'
[System.Windows.Controls.Grid]::SetRow($contentFrame, 2)

$scrollViewer = New-Object System.Windows.Controls.ScrollViewer
$scrollViewer.VerticalScrollBarVisibility = 'Auto'
$scrollViewer.HorizontalScrollBarVisibility = 'Disabled'

$card = New-Object System.Windows.Controls.StackPanel
$card.Background = '#ffffff'
$card.Padding = '16'

$msgLbl = New-Object System.Windows.Controls.TextBlock
$msgLbl.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${msgLabel_b64}'))
$msgLbl.FontSize = 12
$msgLbl.FontWeight = 'Bold'
$msgLbl.Foreground = '#212529'
$card.AddChild($msgLbl)

$msg = New-Object System.Windows.Controls.TextBlock
$msg.Text = '${lastMessage.replace(/'/g, "''").substring(0, 500)}'
$msg.FontFamily = 'Consolas'
$msg.FontSize = 12
$msg.Foreground = '#495057'
$msg.Margin = '0,4,0,0'
$msg.TextWrapping = 'Wrap'
$card.AddChild($msg)

$scrollViewer.Content = $card
$contentFrame.Child = $scrollViewer
$grid.AddChild($contentFrame)
` : ''}

# Button
$btnPanel = New-Object System.Windows.Controls.StackPanel
$btnPanel.HorizontalAlignment = 'Center'
$btnPanel.Margin = '16,12,16,16'
[System.Windows.Controls.Grid]::SetRow($btnPanel, 3)

$btn = New-Object System.Windows.Controls.Button
$btn.Content = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('${btnText_b64}'))
$btn.Width = 120
$btn.Height = 36
$btn.Background = '#4361ee'
$btn.Foreground = '#ffffff'
$btn.FontWeight = 'Bold'
$btn.FontSize = 10
$btn.BorderThickness = '0'
$btn.Cursor = 'Hand'
$btn.Add_Click({ $w.Close() })
$btnPanel.AddChild($btn)

$grid.AddChild($btnPanel)

$w.Content = $grid

$w.Add_Loaded({
    $w.Topmost = $true
    $w.Activate()
    $btn.Focus()
    $w.Topmost = $false
})

$w.Add_KeyDown({
    param($sender, $e)
    if ($e.Key -eq 'Escape' -or $e.Key -eq 'Return') { $w.Close() }
})

$w.ShowDialog() | Out-Null
`;

  try {
    executePowerShellScript(psScript);
  } catch (err) {
    logError(`[Stop] Error: ${err.message}\n`);
  }
}

// ============================================
// PostToolUse 事件处理
// ============================================

function handlePostToolUse(data) {
  const toolName = extractField(data, 'tool_name');

  if (toolName === TOOL_ASK_USER_QUESTION) {
    playSound(SOUND_ASTERISK);
  }
}

// ============================================
// 主入口
// ============================================

let inputData = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => inputData += chunk);
process.stdin.on('end', () => {
  try {
    if (!inputData) {
      logError('[main] No input data received\n');
      return;
    }

    const data = JSON.parse(inputData);
    const event = data.hook_event_name || '';

    if (event === EVENT_PERMISSION_REQUEST) {
      handlePermissionRequest(data);
    } else if (event === EVENT_STOP) {
      handleStop(data);
    } else if (event === EVENT_PRE_TOOL_USE) {
      handlePreToolUse(data);
    } else if (event === EVENT_POST_TOOL_USE) {
      handlePostToolUse(data);
    }
  } catch (err) {
    logError(`[error] ${err.stack}\n`);
  }
});
