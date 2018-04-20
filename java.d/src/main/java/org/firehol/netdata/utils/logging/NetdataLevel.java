package org.firehol.netdata.utils.logging;

import java.util.logging.Level;

/**
 * Log levels defined by Netdata for {@code error.log} (plugin's stderr).
 * 
 * @see <a href="https://github.com/firehol/netdata/wiki/Log-Files#errorlog">Log-Files in Netdata wiki</a>
 */
public class NetdataLevel extends Level {

	private static final long serialVersionUID = 5707091153615417010L;

	protected NetdataLevel(String name, int value) {
		super(name, value);
	}

	/**
	 * Something prevented a program from running.
	 * 
	 * <p>
	 * The log line includes errno (if it is not zero) and the program exited.
	 * 
	 * @see <a href="https://github.com/firehol/netdata/wiki/Log-Files#errorlog">Log-Files in Netdata wiki</a>
	 */
	public static Level FATAL = new NetdataLevel("FATAL", Level.SEVERE.intValue());

	/**
	 * Something that might disable a part of netdata.
	 * 
	 * @see <a href="https://github.com/firehol/netdata/wiki/Log-Files#errorlog">Log-Files in Netdata wiki</a>
	 */
	public static Level ERROR = new NetdataLevel("ERROR", (Level.SEVERE.intValue() + Level.WARNING.intValue())/2);

	/**
	 * Something important the user should know.
	 * 
	 * @see <a href="https://github.com/firehol/netdata/wiki/Log-Files#errorlog">Log-Files in Netdata wiki</a>
	 */
	public static Level INFO = Level.INFO;

}