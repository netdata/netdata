package org.firehol.netdata.test.learning;

import static org.junit.Assert.assertFalse;

import java.io.IOException;
import java.util.Properties;

import org.junit.Ignore;
import org.junit.Test;

import com.sun.tools.attach.AttachNotSupportedException;
import com.sun.tools.attach.VirtualMachine;
import com.sun.tools.attach.VirtualMachineDescriptor;

public class VirtualMachineLearningTest {

	@Test
	@Ignore // This test just produced output to inspect by humans. It does not
			// verify something.
	public void testPrintLocalVirtualMachineSystemProperties() throws AttachNotSupportedException, IOException {
		assertFalse(VirtualMachine.list().isEmpty());
		for (VirtualMachineDescriptor vmDescriptor : VirtualMachine.list()) {
			System.out.println("Display Name: '" + vmDescriptor.displayName() + "'");

			VirtualMachine vm = VirtualMachine.attach(vmDescriptor);
			Properties agentProperties = vm.getAgentProperties();
			assertFalse(agentProperties.isEmpty());
			System.out.println("\nAgent Properties:");
			agentProperties.list(System.out);

			Properties systemProperties = vm.getSystemProperties();
			assertFalse(systemProperties.isEmpty());
			System.out.println("\nSystem Properties:");
			systemProperties.list(System.out);

			System.out.println("\n");

			vm.detach();
		}
	}

}
