#!/usr/bin/python3
import json,os,sys
root=os.path.dirname(os.path.abspath(__file__))
backend=os.path.basename(__file__).replace('fake-','')
fixtures=FIXTURES+'/'+backend
args=sys.argv[1:]
stdin=sys.stdin.read()
with open(root+'/'+backend+'-calls.jsonl','a') as f:
    f.write(json.dumps({'args':args,'cwd':os.getcwd(),'env':sorted(k for k in os.environ if k.startswith(('CLAUDE','CODEX','PI_','RUST_LOG')))})+'\n')
modes=root+'/'+backend+'-modes'
mode='success'
if os.path.exists(modes):
    queue=open(modes).read().split()
    if queue:
        mode=queue[0]
        open(modes,'w').write(' '.join(queue[1:] or queue[-1:]))
def replay(name):
    import datetime,re,time
    reset_file=root+'/reset-in-'+backend
    at=int(time.time())+int(open(reset_file).read() if os.path.exists(reset_file) else 3600)
    local=datetime.datetime.fromtimestamp(at)
    day=local.day
    suffix='th' if 10<=day%100<=20 else {1:'st',2:'nd',3:'rd'}.get(day%10,'th')
    out=open(fixtures+'/'+name+'.stdout').read()
    out=re.sub(r'"resetsAt": ?\d+','"resetsAt":%d'%at,out)
    out=re.sub(r'try again at [^"]*','try again at %s %d%s, %d %s.'%(local.strftime('%b'),day,suffix,local.year,local.strftime('%I:%M %p').lstrip('0')),out)
    sys.stdout.write(out)
    sys.stderr.write(open(fixtures+'/'+name+'.stderr').read())
    sys.exit(int(open(fixtures+'/'+name+'.exit').read()))
if mode!='success':
    if os.path.exists(root+'/partial-'+backend):
        with open('sample.txt','w') as f: f.write(open(root+'/partial-'+backend).read())
    replay(mode)
def verdict():
    if os.path.exists(root+'/edit-'+backend):
        with open('sample.txt','w') as f: f.write(open(root+'/edit-'+backend).read())
        return 'done'
    b=json.loads(stdin)
    v={'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[]}
    if 'documentation' in b['Contract']:
        v['documentation']={'status':'not-needed','reason':'fixture changes no public behavior'}
    if os.path.exists(root+'/findings-'+backend):
        v['verdict']='findings'
        v['findings']=[{'id':'x','priority':'P1','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Sample is wrong','action':'Fix the sample','rule':'','head':b['Head']}]
    return json.dumps(v)
model=args[args.index('--model' if '--model' in args else '-m')+1]
if os.path.exists(root+'/report-model-'+backend):
    model=open(root+'/report-model-'+backend).read()
text=verdict()
if os.path.exists(root+'/bad-verdict-'+backend):
    text='{"verdict":"not a contract answer"}'
if backend=='claude':
    for line in open(fixtures+'/success.stdout'):
        event=json.loads(line)
        if event.get('type')=='system' and event.get('subtype')=='init':
            event['model']=open(root+'/init-model-claude').read() if os.path.exists(root+'/init-model-claude') else model
            event['tools']=args[args.index('--tools')+1].split(',')+(['StructuredOutput'] if '--json-schema' in args else [])
            if os.path.exists(root+'/extra-tool-claude'):
                event['tools'].append(open(root+'/extra-tool-claude').read())
        if event.get('type')=='result':
            event['result']=text
            event['modelUsage']={model:list(event['modelUsage'].values())[0]}
            if os.path.exists(root+'/extra-model-claude'):
                event['modelUsage'][open(root+'/extra-model-claude').read()]=list(event['modelUsage'].values())[0]
            if '--json-schema' in args:
                event['structured_output']=json.loads(text)
            else:
                event.pop('structured_output',None)
        if event.get('type')=='assistant':
            event['message']['model']=model
        print(json.dumps(event))
else:
    for line in open(fixtures+'/success.stdout'):
        event=json.loads(line)
        if event.get('type')=='item.completed':
            event['item']['text']=text
        print(json.dumps(event))
    sys.stderr.write(open(fixtures+'/success.stderr').read().replace('model: "gpt-5.6-luna"','model: "'+model+'"'))
