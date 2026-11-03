"""Result validation"""

from typing import Dict, Any, Optional


class Validator:
    """Validate test results against expected outcomes"""

    def validate_command_result(
        self, actual: Dict[str, Any], expected: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Validate command execution result"""
        result = {
            'valid': True,
            'details': {}
        }

        if actual.get('returncode', -1) != 0:
            result['valid'] = False
            result['details']['exit_code'] = actual.get('returncode')
            result['details']['reason'] = 'Command returned non-zero exit code'

        return result

    def validate_api_response(
        self, actual: Dict[str, Any], expected_status: int = 200
    ) -> Dict[str, Any]:
        """Validate API response"""
        result = {
            'valid': True,
            'details': {}
        }

        status_code = actual.get('status_code')
        if status_code != expected_status:
            result['valid'] = False
            result['details']['status_code'] = status_code
            result['details']['expected_status'] = expected_status
            result['details']['reason'] = f'Expected status {expected_status}, got {status_code}'

        return result

    def validate_service_state(
        self, output: str, service_name: str = 'netdata'
    ) -> Dict[str, Any]:
        """Validate service state"""
        result = {
            'valid': True,
            'details': {}
        }

        service_lower = service_name.lower()
        if service_lower in output.lower():
            if 'active (running)' not in output.lower():
                result['valid'] = False
                result['details']['reason'] = f'{service_name} is not running'
        else:
            result['valid'] = False
            result['details']['reason'] = f'Service {service_name} not found in output'

        return result

    def validate_output_contains(
        self, output: str, expected_text: str, case_sensitive: bool = False
    ) -> Dict[str, Any]:
        """Validate output contains expected text"""
        result = {
            'valid': True,
            'details': {}
        }

        check_output = output if case_sensitive else output.lower()
        check_text = expected_text if case_sensitive else expected_text.lower()

        if check_text not in check_output:
            result['valid'] = False
            result['details']['reason'] = f'Expected text not found in output'
            result['details']['expected'] = expected_text

        return result

    def validate_file_created(self, ssh_client, file_path: str) -> Dict[str, Any]:
        """Validate file was created"""
        result = {
            'valid': True,
            'details': {}
        }

        exists = ssh_client.file_exists(file_path)
        if not exists:
            result['valid'] = False
            result['details']['reason'] = f'File {file_path} was not created'

        return result
