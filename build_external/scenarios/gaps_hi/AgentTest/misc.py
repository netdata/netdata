import subprocess as subp
def sh(cmdline, output, inputtxt=None):
  commonArgs = { 'stdout':subp.PIPE, 'stderr'    :subp.PIPE,
                 'shell' :True} #,      'executable':'/usr/local/bin/bash' }

  print(f"> {cmdline}", file=output)
  if inputtxt is None:
    o = subp.Popen(cmdline, **commonArgs) # nosec
  else:
    o = subp.Popen(cmdline, stdin=subp.PIPE, **commonArgs) # nosec
    o.stdin.write(inputtxt)

  res = o.communicate() # nosec
  if len(res[1])>0:
    output.write( res[1].decode('utf-8') )
  return res[0]

