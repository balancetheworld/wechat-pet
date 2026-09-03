/* 原型数据 1:1 搬移（离线展示用，API_ENABLED=true 后由 services 层数据替代） */

import type { AskPreset, CalRecord, CalTodo, PetRecord, FamilyData } from './types'

export const ASK_PETS = [
  { id: 'all', name: '全部动物', info: '不限宠物，通用提问' },
  { id: 'dou', name: '豆豆', info: '金毛 · 公 · 3岁' },
  { id: 'mi', name: '咪咪', info: '英短 · 母 · 2岁' },
  { id: 'tuan', name: '团子', info: '荷兰侏儒兔 · 1岁' },
]

export const ASK_PRESETS: AskPreset[] = [
  { q: '豆豆下次疫苗什么时候打？', a: '根据日历记录，豆豆的狂犬疫苗下一次提醒日是2026年8月20日，距今还有6天，到期会自动提醒你。上次接种是2025年8月25日，此次为年度加强针，接种后建议留院观察30分钟。' },
  { q: '咪咪最近驱虫做了吗？', a: '咪咪最近一次体外驱虫是8月14日，记录显示使用的是滴剂，状态良好。体内驱虫上次是6月20日，按每3个月一次的建议，下次体内驱虫应在9月20日左右，届时日历会生成提醒。' },
  { q: '豆豆体重正常吗？', a: '豆豆目前体重28kg，处于金毛犬标准体重范围内（25-34kg），体态健康。日常建议每天喂食优质成犬粮约300-350g，分早晚两餐，并保持适量运动，避免肥胖增加关节负担。' },
  { q: '新手养猫要注意什么？', a: '新手养猫注意以下几点：1. 接种疫苗（猫三联+狂犬）；2. 定期驱虫（体外每月、体内每3个月）；3. 提供干净的猫砂盆并每日清理；4. 不要喂牛奶、巧克力、洋葱等；5. 封窗防止坠楼；6. 每年至少体检一次。有具体问题可以继续问我~' },
  { q: '团子能吃胡萝卜吗？', a: '可以吃。胡萝卜是兔子可以食用的蔬菜之一，富含维生素A，但糖分偏高，建议作为零食少量喂食（每次一小片），主食仍应以无限量提摩西草+适量兔粮为主。喂前洗净擦干，避免腹泻。' },
]

export const ASK_MOCKS = [
  '根据豆豆的健康档案记录，豆豆是一只3岁的金毛寻回犬。最后一次体检是在2026年7月10日，各项指标正常。目前疫苗接种齐全，下次狂犬疫苗提醒日是8月20日。建议保持每月一次的体外驱虫频率。',
  '豆豆目前体重28kg，处于金毛犬标准体重范围内（25-34kg），体态健康。日常饮食建议每天喂食优质成犬粮约300-350g，分早晚两餐。可以适量补充鸡胸肉、蛋黄等蛋白质，但注意不要喂巧克力、葡萄、洋葱等对狗有毒的食物。',
  '豆豆的日常记录来看，豆豆最近学会了"握手"这个技能，非常聪明！金毛犬智商排名第四，学习能力强。建议继续用正向奖励训练法，每次训练控制在10-15分钟内，配合零食奖励效果更佳。',
  '关于这个问题，我需要更多信息来给出准确回答。你可以告诉我具体出现了什么症状吗？比如食欲、精神状态、排便情况等。如果情况紧急，建议直接联系你的宠物医生。',
  '咪咪的驱虫记录显示，最近一次体外驱虫是8月14日，使用的是滴剂。按照每月一次的频率，下一次驱虫时间大约在9月中旬。体内驱虫建议每3个月一次，上次是6月进行的。',
]

export const CAL_PETS = [
  { id: 'dou', name: '豆豆', emoji: '🐶' },
  { id: 'mi', name: '咪咪', emoji: '🐱' },
  { id: 'tuan', name: '团子', emoji: '🐹' },
]
export const CAL_MEMBERS: Record<string, { name: string }> = { ma: { name: '妈妈' }, ba: { name: '爸爸' } }

export const INIT_CAL_RECORDS: CalRecord[] = [
  { date: '2026-08-14', pet: 'mi', cat: 'medical', medType: '驱虫', title: '体外驱虫', text: '今天给咪咪做了体外驱虫，医生说状态很好。', imgs: ['🧴', '🩺'], by: 'ma', time: '10:20' },
  { date: '2026-08-14', pet: 'dou', cat: 'daily', medType: null, title: '学会握手啦', text: '豆豆今天终于学会握手了！奖励了最爱的小零食。', imgs: ['🐾', '🍪'], by: 'ba', time: '18:05' },
  { date: '2026-08-03', pet: 'dou', cat: 'medical', medType: '疫苗', title: '狂犬疫苗第一针', text: '接种完成，留观30分钟无异常。', imgs: ['💉'], by: 'ma', time: '14:30' },
  { date: '2026-08-05', pet: 'tuan', cat: 'daily', medType: null, title: '换了新草窝', text: '团子对新草窝爱不释口，滚来滚去。', imgs: ['🌿', '🐹'], by: 'ba', time: '09:15' },
  { date: '2026-08-10', pet: 'mi', cat: 'daily', medType: null, title: '晒太阳的小团子', text: '咪咪趴在窗台上晒了一下午太阳。', imgs: ['☀️', '😻'], by: 'ma', time: '16:40' },
]

export const INIT_CAL_TODOS: CalTodo[] = [
  { id: 't1', pet: 'dou', title: '狂犬疫苗第二针', remindDate: '2026-08-20', cycle: 'yearly', cycleName: '每年', status: 'pending', by: 'ma' },
]

/* 档案册（书）数据 */
export const PETS: PetRecord[] = [
  {
    id: 'milo',
    name: '小猫',
    title: '小猫的成长小册',
    quote: '小小一团，却把整个家都装进了心里。',
    years: '2021 — 至今',
    type: '英国短毛猫<br>女生 · 已绝育',
    birthDate: '2021-12-18',
    arrivalDate: '2022-02-04',
    tags: ['慢热', '粘人', '好奇', '挑食', '需要独处'],
    personalityQuestions: [
      { title: '它喜欢什么', summary: '晒过太阳的窗台、羽毛逗猫棒和纸箱', detail: '羽毛逗猫棒、刚拆开的纸箱，还有家人下班时钥匙开门的声音。' },
      { title: '它害怕什么', summary: '吹风机和突然的巨大声响', detail: '吹风机一开就会躲到沙发下面，安静后要等一会儿才愿意出来。' },
      { title: '它有哪些生活习惯', summary: '早上等门，下午睡书架，晚上催睡觉', detail: '早上会在卧室门口等人，下午睡书架，晚上十一点准时催大家睡觉。' },
      { title: '和它相处时需要注意', summary: '先让它闻闻手，不要突然抱起', detail: '陌生人不要直接摸头。蹲下等它主动靠近，会更容易成为朋友。' },
      { title: '我们眼中的他', summary: '一位认真又温柔的家庭观察员', detail: '看起来总是很淡定，其实每一件家务它都要坐在旁边认真监督。' },
    ],
    health: {
      overall: '稳定 · 已绝育',
      allergies: { label: '过敏信息', value: '1 项', note: '鸡肉蛋白' },
      diseases: { label: '既往疾病', value: '1 条', note: '已恢复' },
      medications: { label: '长期用药', value: '无', note: '目前未使用' },
      vaccines: { label: '最近疫苗', value: '猫三联', note: '2025.10.12' },
    },
    updated: '小满 修改于 2026.07.21 20:18',
    backFamily: '木棉之家 · 更新于 2026年8月13日',
    weights: [{ date: '2026-08-06', value: '4.7' }, { date: '2026-07-05', value: '4.8' }, { date: '2026-05-20', value: '4.6' }],
    events: [
      { type: '日常趣事', date: '2026-08-12', title: '纸箱巡视员上岗', content: '新快递刚到，小猫已经先一步检查完毕。', author: '小满' },
      { type: '成长里程碑', date: '2026-07-28', title: '学会自己开推拉门', content: '观察半个月后，终于完成了第一次独立开门。', author: '阿岚' },
      { type: '家庭共同回忆', date: '2026-07-09', title: '一起晒夏天的太阳', content: '小猫和小狗难得并排睡了一整个下午。', author: '小满' },
      { type: '换牙记录', date: '2022-07-13', title: '换牙完成', content: '最后一颗小乳牙也顺利毕业了。', author: '小满' },
    ],
    birthdays: [
      { date: '2025-12-18', age: '4岁', title: '今天也被很多爱包围', wish: '要继续健康、自在地长大呀。', mediaLabel: '2 张照片 · 1 个视频' },
      { date: '2024-12-18', age: '3岁', title: '第一次一起去露营', wish: '新的一岁继续一起看更多风景。', mediaLabel: '1 张照片' },
      { date: '2023-12-18', age: '2岁', title: '窗边的小派对', wish: '谢谢你来到我们家。', mediaLabel: '1 张照片' },
    ],
  },
  {
    id: 'doubao',
    name: '小狗',
    title: '小狗的成长小册',
    quote: '每天回家，都有一个小太阳在等我。',
    years: '2020 — 至今',
    type: '金毛寻回犬<br>男生 · 已绝育',
    birthDate: '2020-05-12',
    arrivalDate: '2020-07-20',
    tags: ['热情', '贪吃', '粘人', '爱玩水', '社交达人'],
    personalityQuestions: [
      { title: '它喜欢什么', summary: '球球、游泳和被摸脑袋', detail: '最爱的玩具是黄色的橡胶球，夏天一定要去河里踩水，还有家人回家时摸摸它的头。' },
      { title: '它害怕什么', summary: '烟花和吸尘器的轰鸣声', detail: '每逢节日放烟花，都会躲到沙发角落，需要人陪着等声音过去。' },
      { title: '它有哪些生活习惯', summary: '早晚各遛一次，饭前先握手', detail: '早上七点准时叫人起床，晚上十点会叼着牵引绳坐在门口。吃饭前必须完成握手口令。' },
      { title: '和它相处时需要注意', summary: '不要从背后突然靠近', detail: '虽然性格温顺，但突然从背后拍它会吓一跳。建议和它对视后再伸手。' },
      { title: '我们眼中的他', summary: '家里的快乐发电机', detail: '只要有它在，家里就永远不会安静。它的尾巴好像装了马达。' },
    ],
    health: {
      overall: '健康 · 已绝育',
      allergies: { label: '过敏信息', value: '无', note: '暂未发现' },
      diseases: { label: '既往疾病', value: '无', note: '体健' },
      medications: { label: '长期用药', value: '无', note: '目前未使用' },
      vaccines: { label: '最近疫苗', value: '狂犬+八联', note: '2025.09.08' },
    },
    updated: '阿岚 修改于 2026.08.10 10:42',
    backFamily: '木棉之家 · 更新于 2026年8月14日',
    weights: [{ date: '2026-08-14', value: '28.0' }, { date: '2026-07-14', value: '28.2' }, { date: '2026-06-10', value: '27.8' }],
    events: [
      { type: '技能记录', date: '2026-08-01', title: '学会叼快递盒', content: '帮妈妈把轻的小快递盒叼到茶几上，成就感满满。', author: '小满' },
      { type: '健康记录', date: '2026-07-15', title: '夏季体检', content: '各项指标正常，医生建议控制零食量。', author: '阿岚' },
      { type: '旅行回忆', date: '2026-06-03', title: '第一次露营', content: '在草地上疯跑了一下午，晚上睡在帐篷门口守夜。', author: '小满' },
      { type: '换牙记录', date: '2020-09-10', title: '乳牙换完', content: '最后一颗乳牙脱落，正式变成大狗狗。', author: '阿岚' },
    ],
    birthdays: [
      { date: '2026-05-12', age: '6岁', title: '湖边生日派对', wish: '希望你永远跑得动、笑得开心。', mediaLabel: '3 张照片 · 2 个视频' },
      { date: '2025-05-12', age: '5岁', title: '收到新球的一年', wish: '继续做最快乐的毛孩子。', mediaLabel: '2 张照片' },
      { date: '2024-05-12', age: '4岁', title: '雨天的生日散步', wish: '每一天都想起你摇尾巴的样子。', mediaLabel: '1 张照片' },
    ],
  },
]

export const CHAPTER_NAMES = ['封面', '身份名片', '个性说明书', '健康资料', '生日纪念册', '成长足迹', '封底']

export const FAMILY: FamilyData = {
  name: '木棉之家',
  code: 'KM2026',
  members: [
    { name: '我', role: '主人', status: 'active', appliedAt: '2026-01-08', reviewedAt: '2026-01-08' },
    { name: '妈妈', role: '管理员', status: 'active', appliedAt: '2026-01-10', reviewedAt: '2026-01-10' },
    { name: '爸爸', role: '成员', status: 'active', appliedAt: '2026-01-12', reviewedAt: '2026-01-12' },
    { name: '表哥', role: '成员', status: 'pending', appliedAt: '2026-08-19', reviewedAt: '' },
    { name: '邻居姐姐', role: '成员', status: 'pending', appliedAt: '2026-08-20', reviewedAt: '' },
    { name: '前室友', role: '成员', status: 'rejected', appliedAt: '2026-08-12', reviewedAt: '2026-08-13' },
  ],
  pets: [
    { id: 'milo', name: '小猫' },
    { id: 'doubao', name: '小狗' },
  ],
}
